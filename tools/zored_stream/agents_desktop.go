package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

type desktopRecord struct {
	SessionID       string          `json:"sessionId"`
	CliSessionID    string          `json:"cliSessionId"`
	Cwd             string          `json:"cwd"`
	Title           string          `json:"title"`
	LastActivityAt  int64           `json:"lastActivityAt"`
	SpawnedFrom     json.RawMessage `json:"spawnedFrom"`
	IsArchived      bool            `json:"isArchived"`
	ScheduledTaskID string          `json:"scheduledTaskId"`
	LastFocusedAt   int64           `json:"lastFocusedAt"`
	LastTurnEdgeAt  int64           `json:"lastTurnEdgeAt"`
}

func (r desktopRecord) unread() bool {
	return !r.IsArchived && r.ScheduledTaskID == "" && r.LastTurnEdgeAt > r.LastFocusedAt
}

func (r desktopRecord) parent() string {
	var from struct {
		SessionID string `json:"sessionId"`
	}
	if !r.child() {
		return ""
	}
	if err := json.Unmarshal(r.SpawnedFrom, &from); err != nil {
		log.Printf("agents: %s spawnedFrom: %v", r.SessionID, err)
	}
	return from.SessionID
}

func (r desktopRecord) child() bool {
	return len(r.SpawnedFrom) > 0 && string(r.SpawnedFrom) != "null"
}

func (r desktopRecord) session() *agentSession {
	id := r.CliSessionID
	if id == "" {
		id = r.SessionID
	}
	return &agentSession{ID: id, Kind: agentDone, Since: time.UnixMilli(r.LastTurnEdgeAt), Cwd: r.Cwd, Host: r.SessionID,
		Updated: time.UnixMilli(max(r.LastActivityAt, r.LastTurnEdgeAt)), Child: r.child()}
}

type desktopFile struct {
	mod  time.Time
	size int64
	rec  desktopRecord
	ok   bool
}

type desktopSessions struct {
	mu    sync.Mutex
	dir   string
	files map[string]desktopFile
	dirs  map[string]time.Time
	recs  map[string]desktopRecord
}

func newDesktopSessions(configDir string) *desktopSessions {
	return &desktopSessions{dir: filepath.Join(configDir, "Claude", "claude-code-sessions"), files: map[string]desktopFile{}}
}

func (d *desktopSessions) scan() (map[string]desktopRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.scanLocked()
}

// scanIfChanged rescans only when a session directory changed: Desktop replaces the files, which bumps the directory mtime.
func (d *desktopSessions) scanIfChanged() (map[string]desktopRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	paths, err := filepath.Glob(filepath.Join(d.dir, "*", "*"))
	if err != nil {
		return nil, err
	}
	same := d.recs != nil && len(paths) == len(d.dirs)
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		same = same && d.dirs[p].Equal(info.ModTime())
	}
	if same {
		return d.recs, nil
	}
	return d.scanLocked()
}

func (d *desktopSessions) record(host string) (desktopRecord, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	rec, ok := d.recs[host]
	return rec, ok
}

func (d *desktopSessions) front() (desktopRecord, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	rec, ok := d.recs[frontDesktop(d.recs)]
	return rec, ok
}

func (d *desktopSessions) scanLocked() (map[string]desktopRecord, error) {
	dirs := map[string]time.Time{}
	dirPaths, err := filepath.Glob(filepath.Join(d.dir, "*", "*"))
	if err != nil {
		return nil, err
	}
	for _, p := range dirPaths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		dirs[p] = info.ModTime()
	}
	paths, err := filepath.Glob(filepath.Join(d.dir, "*", "*", "local_*.json"))
	if err != nil {
		return nil, err
	}
	files := make(map[string]desktopFile, len(paths))
	recs := make(map[string]desktopRecord, len(paths))
	for _, p := range paths {
		info, err := os.Stat(p)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		f, cached := d.files[p]
		if !cached || !f.mod.Equal(info.ModTime()) || f.size != info.Size() {
			f = desktopFile{mod: info.ModTime(), size: info.Size()}
			b, err := os.ReadFile(p)
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return nil, err
			}
			if err == nil {
				if err := json.Unmarshal(b, &f.rec); err != nil {
					log.Printf("agents: desktop session %s: %v", filepath.Base(p), err)
				} else {
					f.ok = true
				}
			}
		}
		files[p] = f
		if f.ok {
			recs[f.rec.SessionID] = f.rec
		}
	}
	d.files, d.dirs, d.recs = files, dirs, recs
	return recs, nil
}

func (d *desktopSessions) family(host string) []desktopRecord {
	d.mu.Lock()
	defer d.mu.Unlock()
	root := host
	for range 16 {
		r, ok := d.recs[root]
		if !ok || r.parent() == "" {
			break
		}
		root = r.parent()
	}
	kids := map[string][]desktopRecord{}
	for _, r := range d.recs {
		if p := r.parent(); p != "" && !r.IsArchived && r.ScheduledTaskID == "" {
			kids[p] = append(kids[p], r)
		}
	}
	var out []desktopRecord
	var walk func(id string, depth int)
	walk = func(id string, depth int) {
		r, ok := d.recs[id]
		if !ok || depth > 16 {
			return
		}
		out = append(out, r)
		k := kids[id]
		slices.SortFunc(k, func(x, y desktopRecord) int {
			return cmp.Or(cmp.Compare(max(y.LastActivityAt, y.LastTurnEdgeAt), max(x.LastActivityAt, x.LastTurnEdgeAt)), strings.Compare(x.SessionID, y.SessionID))
		})
		for _, c := range k {
			walk(c.SessionID, depth+1)
		}
	}
	walk(root, 0)
	return out
}
