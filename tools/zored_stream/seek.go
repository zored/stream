package main

import (
	"errors"
	"log"
	"sync"
	"time"
)

const (
	seekEvery  = 150 * time.Millisecond
	seekMargin = 1.0
)

var seekSeconds = [...]float64{2, 4, 8, 15}

type seeker struct {
	mu      sync.Mutex
	np      *nowPlaying
	bar     *bar
	accel   accel
	target  float64
	until   time.Time
	sent    time.Time
	pending bool
}

func (s *seeker) turn(direction int) error {
	t := s.np.snapshot()
	if t.Duration <= 0 {
		return errors.New("now playing has no duration")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if now.After(s.until) {
		s.target = t.position(now)
	}
	amount := s.accel.step(direction)
	sign := 1.0
	if amount < 0 {
		amount, sign = -amount, -1
	}
	s.target = max(0, min(t.Duration-seekMargin, s.target+sign*seekSeconds[amount-1]))
	s.until = now.Add(visibleFor)
	s.bar.show("seek", s.target/t.Duration*100)
	if s.pending {
		return nil
	}
	if wait := seekEvery - now.Sub(s.sent); wait > 0 {
		s.pending = true
		time.AfterFunc(wait, s.flush)
		return nil
	}
	return s.sendLocked()
}

func (s *seeker) flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = false
	if err := s.sendLocked(); err != nil {
		log.Printf("seek: %v", err)
	}
}

func (s *seeker) sendLocked() error {
	s.sent = time.Now()
	if err := s.np.seek(s.target); err != nil {
		return err
	}
	s.np.publishProgress()
	return nil
}
