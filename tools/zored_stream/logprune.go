package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const logPruneEvery = time.Hour

type logPruner struct {
	dir  string
	keep time.Duration
}

func (p logPruner) prune() {
	if err := p.pruneAt(time.Now()); err != nil {
		log.Printf("logprune: %v", err)
	}
}

func (p logPruner) pruneAt(now time.Time) error {
	entries, err := os.ReadDir(p.dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "companion-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		if now.Sub(info.ModTime()) <= p.keep {
			continue
		}
		if err := os.Remove(filepath.Join(p.dir, name)); err != nil {
			return err
		}
		log.Printf("logprune: removed %s", name)
	}
	return nil
}
