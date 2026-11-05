package main

import (
	"log"
	"sync"
	"time"
)

const trackKnobHold = 400 * time.Millisecond

// trackKnob: turn seeks, press + turn skips tracks, a click plays / pauses, a hold likes.
type trackKnob struct {
	mu      sync.Mutex
	pressed bool
	used    bool
	hold    *time.Timer
	seek    func(direction int) error
	skip    func(direction int) error
	play    func() error
	like    func() error
}

func (k *trackKnob) down() error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.pressed, k.used = true, false
	k.hold = time.AfterFunc(trackKnobHold, func() {
		k.mu.Lock()
		if !k.pressed || k.used {
			k.mu.Unlock()
			return
		}
		k.used = true
		k.mu.Unlock()
		if err := k.like(); err != nil {
			log.Printf("track knob: like: %v", err)
		}
	})
	return nil
}

func (k *trackKnob) up() error {
	k.mu.Lock()
	if k.hold != nil {
		k.hold.Stop()
	}
	used := k.used
	k.pressed = false
	k.mu.Unlock()
	if used {
		return nil
	}
	return k.play()
}

func (k *trackKnob) turn(direction int) error {
	k.mu.Lock()
	pressed := k.pressed
	if pressed {
		k.used = true
	}
	k.mu.Unlock()
	if pressed {
		return k.skip(direction)
	}
	return k.seek(direction)
}
