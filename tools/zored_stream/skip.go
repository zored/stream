package main

import (
	"fmt"
	"strconv"
	"sync"
	"time"
)

const (
	skipFor  = 800 * time.Millisecond
	skipFrom = -90.0
	skipTo   = 110.0
)

type skip struct {
	mu        sync.Mutex
	direction int
	started   time.Time
	wake      chan struct{}
	vars      *vars
}

func newSkip(v *vars) *skip {
	return &skip{wake: make(chan struct{}, 1), vars: v}
}

func (s *skip) show(direction int) {
	s.mu.Lock()
	s.direction, s.started = direction, time.Now()
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *skip) animate() {
	for range s.wake {
		ticker := time.NewTicker(time.Second / animationHz)
		for range ticker.C {
			if !s.frame() {
				break
			}
		}
		ticker.Stop()
	}
}

func (s *skip) frame() bool {
	s.mu.Lock()
	p := min(1, float64(time.Since(s.started))/float64(skipFor))
	direction := s.direction
	if p == 1 {
		s.direction = 0
	}
	s.mu.Unlock()
	eased := p * p * (3 - 2*p)
	if direction < 0 {
		eased = 1 - eased
	}
	s.vars.set("np_skip_x", fmt.Sprintf("%.1f", skipFrom+(skipTo-skipFrom)*eased))
	if p == 1 {
		direction = 0
	}
	s.vars.set("np_skip", strconv.Itoa(direction))
	return p < 1
}
