package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLogPrunerRemovesOnlyOldCompanionLogs(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	files := map[string]time.Duration{
		"companion-2026-11-17.6.log": 5 * 24 * time.Hour,
		"companion-2026-11-24.0.log": time.Hour,
		"audit.json":                 30 * 24 * time.Hour,
		"other-old.log":              30 * 24 * time.Hour,
	}
	for name, age := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, now.Add(-age), now.Add(-age)); err != nil {
			t.Fatal(err)
		}
	}
	if err := (logPruner{dir: dir, keep: 3 * 24 * time.Hour}).pruneAt(now); err != nil {
		t.Fatal(err)
	}
	for name := range files {
		_, err := os.Stat(filepath.Join(dir, name))
		gone := os.IsNotExist(err)
		if want := name == "companion-2026-11-17.6.log"; gone != want {
			t.Errorf("%s removed=%v, want %v", name, gone, want)
		}
	}
}
