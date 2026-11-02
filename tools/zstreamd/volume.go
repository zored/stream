package main

import (
	"strconv"
	"sync"
	"time"
)

type volume struct {
	mu        sync.Mutex
	value     int
	muted     bool
	lastTick  time.Time
	direction int
	vars      *vars
}

func (v *volume) poll() error {
	value, muted, err := readAudio()
	if err != nil {
		return err
	}
	v.mu.Lock()
	v.value, v.muted = value, muted
	v.mu.Unlock()
	v.publish()
	return nil
}

func (v *volume) publish() {
	v.mu.Lock()
	text := strconv.Itoa(v.value)
	if v.muted {
		text = "MUTE"
	}
	v.mu.Unlock()
	v.vars.set("volume", text)
}

func (v *volume) step(direction int) error {
	v.mu.Lock()
	now := time.Now()
	dt := time.Second
	if direction == v.direction {
		dt = now.Sub(v.lastTick)
	}
	v.lastTick, v.direction = now, direction
	amount := 4
	switch {
	case dt > 200*time.Millisecond:
		amount = 1
	case dt > 100*time.Millisecond:
		amount = 2
	case dt > 50*time.Millisecond:
		amount = 3
	}
	err := v.setLocked(v.value + direction*amount)
	v.mu.Unlock()
	v.publish()
	return err
}

func (v *volume) setLocked(value int) error {
	v.value = max(0, min(100, value))
	if err := writeVolume(v.value); err != nil {
		return err
	}
	if v.muted && v.value > 0 {
		v.muted = false
		return writeMute(false)
	}
	return nil
}

func (v *volume) toggleMute() error {
	v.mu.Lock()
	v.muted = !v.muted
	err := writeMute(v.muted)
	v.mu.Unlock()
	v.publish()
	return err
}
