package main

import "sync"

var scrollLines = [...]int{1, 2, 4, 8}

type scroller struct {
	mu         sync.Mutex
	accel      accel
	horizontal bool
}

func (s *scroller) hold(down bool) {
	s.mu.Lock()
	s.horizontal = down
	s.mu.Unlock()
}

func (s *scroller) turn(direction int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lines := -direction * scrollLines[s.accel.step(direction)*direction-1]
	if s.horizontal {
		return postScroll(0, lines)
	}
	return postScroll(lines, 0)
}
