package main

import "sync"

type mode struct {
	name   string
	active func() bool
	close  func()
}

// modes keeps one temporary tile mode at a time: one that opens closes every other open one.
type modes struct {
	mu   sync.Mutex
	list []mode
	was  map[string]bool
}

var tileModes = &modes{was: map[string]bool{}}

func (m *modes) add(md mode) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.list = append(m.list, md)
}

func (m *modes) check() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := map[string]bool{}
	opened := false
	for _, md := range m.list {
		now[md.name] = md.active()
		opened = opened || now[md.name] && !m.was[md.name]
	}
	if opened {
		for _, md := range m.list {
			if now[md.name] && m.was[md.name] {
				md.close()
				now[md.name] = false
			}
		}
	}
	m.was = now
}
