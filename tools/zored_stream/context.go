package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	contextTail      = 512 << 10
	contextWindow    = 1_000_000
	contextWindowLow = 200_000
	contextBarH      = 4
)

type chatContext struct {
	Tokens  int     `json:"tokens"`
	Window  int     `json:"window"`
	Percent float64 `json:"percent"`
	Model   string  `json:"model"`
}

type contextFile struct {
	path string
	mod  time.Time
	size int64
	ctx  chatContext
}

type contexts struct {
	mu       sync.Mutex
	projects string
	files    map[string]contextFile
}

func newContexts(home string) *contexts {
	return &contexts{projects: filepath.Join(home, ".claude", "projects"), files: map[string]contextFile{}}
}

func (c *contexts) of(id string) (chatContext, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	f, ok := c.files[id]
	if !ok {
		paths, err := filepath.Glob(filepath.Join(c.projects, "*", id+".jsonl"))
		if err != nil {
			return chatContext{}, err
		}
		if len(paths) == 0 {
			return chatContext{}, fmt.Errorf("no transcript for %s", id)
		}
		f.path = paths[0]
	}
	info, err := os.Stat(f.path)
	if err != nil {
		delete(c.files, id)
		return chatContext{}, err
	}
	if ok && info.ModTime().Equal(f.mod) && info.Size() == f.size {
		return f.ctx, nil
	}
	ctx, err := readContext(f.path, info.Size())
	if err != nil {
		return chatContext{}, err
	}
	c.files[id] = contextFile{path: f.path, mod: info.ModTime(), size: info.Size(), ctx: ctx}
	return ctx, nil
}

func readContext(path string, size int64) (chatContext, error) {
	file, err := os.Open(path)
	if err != nil {
		return chatContext{}, err
	}
	defer file.Close()
	from := max(0, size-contextTail)
	buf := make([]byte, size-from)
	if _, err := file.ReadAt(buf, from); err != nil && err != io.EOF {
		return chatContext{}, err
	}
	lines := bytes.Split(buf, []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		if !bytes.Contains(lines[i], []byte(`"usage"`)) {
			continue
		}
		var entry struct {
			Message struct {
				Model string `json:"model"`
				Usage *struct {
					Input       int `json:"input_tokens"`
					CacheRead   int `json:"cache_read_input_tokens"`
					CacheCreate int `json:"cache_creation_input_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if json.Unmarshal(lines[i], &entry) != nil || entry.Message.Usage == nil || entry.Message.Model == "<synthetic>" {
			continue
		}
		u := entry.Message.Usage
		tokens := u.Input + u.CacheRead + u.CacheCreate
		window := contextWindowFor(entry.Message.Model)
		return chatContext{Tokens: tokens, Window: window, Percent: 100 * float64(tokens) / float64(window), Model: entry.Message.Model}, nil
	}
	return chatContext{}, fmt.Errorf("no usage in the last %d KB of %s", contextTail>>10, filepath.Base(path))
}

func contextWindowFor(model string) int {
	if strings.Contains(model, "haiku") {
		return contextWindowLow
	}
	return contextWindow
}
