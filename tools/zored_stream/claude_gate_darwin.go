package main

import (
	"sync"
	"time"
)

const (
	claudeHotFor    = 20 * time.Second
	claudeColdEvery = 3 * time.Second
)

// claudeGate keeps AX walks of the Claude window at full rate only while something there may change.
type claudeGate struct {
	mu    sync.Mutex
	poke  time.Time
	front int
}

var claude claudeGate

func (g *claudeGate) wake() {
	g.mu.Lock()
	g.poke = time.Now()
	g.mu.Unlock()
}

func (g *claudeGate) due(last *time.Time) bool {
	now, front := time.Now(), int(frontPid())
	g.mu.Lock()
	if front != g.front {
		g.front, g.poke = front, now
	}
	hot := now.Sub(g.poke) < claudeHotFor
	g.mu.Unlock()
	if !hot && now.Sub(*last) < claudeColdEvery {
		return false
	}
	*last = now
	return true
}
