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

const tabRelease = 500 * time.Millisecond

var scrollLines = [...]int{1, 2, 4, 8}

type scroller struct {
	mu      sync.Mutex
	accel   accel
	file    string
	tab     bool
	pressed bool
	turned  bool
	ctrl    bool
	timer   *time.Timer
}

func newScroller(file string) (*scroller, error) {
	s := &scroller{file: file}
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
	return s, postCtrl(false)
}

func (s *scroller) hold(down bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if down {
		s.pressed, s.turned = true, false
		return s.releaseLocked()
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
		return postScroll(0, s.lines(direction))
	}
	if !s.tab {
		return postScroll(s.lines(direction), 0)
	}
	if !s.ctrl {
		if err := postCtrl(true); err != nil {
			return err
		}
		s.ctrl = true
	}
	if s.timer == nil {
		s.timer = time.AfterFunc(tabRelease, s.release)
	} else {
		s.timer.Reset(tabRelease)
	}
	return postTab(direction < 0)
}

func (s *scroller) lines(direction int) int {
	return -direction * scrollLines[s.accel.step(direction)*direction-1]
}

func (s *scroller) release() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.releaseLocked(); err != nil {
		log.Printf("release ctrl: %v", err)
	}
}

func (s *scroller) releaseLocked() error {
	if s.timer != nil {
		s.timer.Stop()
	}
	if !s.ctrl {
		return nil
	}
	s.ctrl = false
	return postCtrl(false)
}
