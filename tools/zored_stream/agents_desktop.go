package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type desktopRecord struct {
	SessionID       string `json:"sessionId"`
	CliSessionID    string `json:"cliSessionId"`
	Cwd             string `json:"cwd"`
	IsArchived      bool   `json:"isArchived"`
	ScheduledTaskID string `json:"scheduledTaskId"`
	LastFocusedAt   int64  `json:"lastFocusedAt"`
	LastTurnEdgeAt  int64  `json:"lastTurnEdgeAt"`
}

func (r desktopRecord) unread() bool {
	return !r.IsArchived && r.ScheduledTaskID == "" && r.LastTurnEdgeAt > r.LastFocusedAt
}

func (r desktopRecord) session() *agentSession {
	id := r.CliSessionID
	if id == "" {
		id = r.SessionID
	}
	return &agentSession{ID: id, Kind: agentDone, Since: time.UnixMilli(r.LastTurnEdgeAt), Cwd: r.Cwd, Host: r.SessionID}
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
