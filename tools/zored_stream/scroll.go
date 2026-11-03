package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	tabRelease = 500 * time.Millisecond
	cmdRelease = 5 * time.Second
)

var scrollLines = [...]int{1, 2, 4, 8}

type modifier int

const (
	modNone modifier = iota
	modCtrl
	modCmd
)

type scroller struct {
	mu      sync.Mutex
	accel   accel
	file    string
	tab     bool
	pressed bool
	turned  bool
	held    modifier
	timer   *time.Timer
	wheel   func(vertical, horizontal int) error
	modKey  func(m modifier, down bool) error
	tabKey  func(m modifier, shift bool) error
}

func newScroller(file string) (*scroller, error) {
	s := &scroller{file: file, wheel: postScroll, modKey: postModifier, tabKey: postTab}
	data, err := os.ReadFile(file)
	switch {
	case os.IsNotExist(err):
	case err != nil:
		return nil, err
	default:
		switch mode := strings.TrimSpace(string(data)); mode {
		case "wheel":
		case "tab":
			s.tab = true
		default:
			return nil, fmt.Errorf("%s: unknown mode %q", file, mode)
		}
	}
	if err := s.modKey(modCtrl, false); err != nil {
		return nil, err
	}
	return s, s.modKey(modCmd, false)
}

func (s *scroller) hold(down bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.releaseLocked(); err != nil {
		return err
	}
	if down {
		s.pressed, s.turned = true, false
		return nil
	}
	if !s.pressed {
		return nil
	}
	s.pressed = false
	if s.turned {
		return nil
	}
	s.tab = !s.tab
	mode := "wheel"
	if s.tab {
		mode = "tab"
	}
	log.Printf("scroll mode %s", mode)
	if err := os.MkdirAll(filepath.Dir(s.file), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.file, []byte(mode+"\n"), 0o644)
}

func (s *scroller) turn(direction int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pressed {
		s.turned = true
		if !s.tab {
			return s.wheel(0, s.lines(direction))
		}
		return s.tabLocked(modCmd, cmdRelease, direction)
	}
	if !s.tab {
		return s.wheel(s.lines(direction), 0)
	}
	return s.tabLocked(modCtrl, tabRelease, direction)
}

func (s *scroller) tabLocked(m modifier, after time.Duration, direction int) error {
	if s.held != m {
		if err := s.releaseLocked(); err != nil {
			return err
		}
		if err := s.modKey(m, true); err != nil {
			return err
		}
		s.held = m
	}
	if s.timer == nil {
		s.timer = time.AfterFunc(after, s.release)
	} else {
		s.timer.Reset(after)
	}
	return s.tabKey(m, direction < 0)
}

func (s *scroller) lines(direction int) int {
	return -direction * scrollLines[s.accel.step(direction)*direction-1]
}

func (s *scroller) release() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.releaseLocked(); err != nil {
		log.Printf("release modifier: %v", err)
	}
}

func (s *scroller) releaseLocked() error {
	if s.timer != nil {
		s.timer.Stop()
	}
	if s.held == modNone {
		return nil
	}
	m := s.held
	s.held = modNone
	return s.modKey(m, false)
}
