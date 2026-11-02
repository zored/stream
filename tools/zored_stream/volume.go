package main

import (
	"fmt"
	"math"
	"strconv"
	"sync"
	"time"
)

const (
	visibleFor          = time.Second
	animationHz         = 30
	easing              = 0.3
	fadeIn              = 0.6
	fadeOut             = 0.25
	dragPercentPerPixel = 0.4
)

type volume struct {
	mu            sync.Mutex
	value         int
	muted         bool
	shown         float64
	alpha         float64
	visibleUntil  time.Time
	dragRemainder float64
	lastTick      time.Time
	direction     int
	wake          chan struct{}
	vars          *vars
}

func newVolume(v *vars) (*volume, error) {
	value, muted, err := readAudio()
	if err != nil {
		return nil, err
	}
	vol := &volume{value: value, muted: muted, wake: make(chan struct{}, 1), vars: v}
	if !muted {
		vol.shown = float64(value)
	}
	return vol, nil
}

func (v *volume) poll() error {
	value, muted, err := readAudio()
	if err != nil {
		return err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if value == v.value && muted == v.muted {
		return nil
	}
	v.value, v.muted = value, muted
	v.revealLocked()
	return nil
}

func (v *volume) revealLocked() {
	v.visibleUntil = time.Now().Add(visibleFor)
	select {
	case v.wake <- struct{}{}:
	default:
	}
}

func (v *volume) animate() {
	for range v.wake {
		ticker := time.NewTicker(time.Second / animationHz)
		for range ticker.C {
			if !v.frame() {
				break
			}
		}
		ticker.Stop()
	}
}

func (v *volume) frame() bool {
	v.mu.Lock()
	target := float64(v.value)
	if v.muted {
		target = 0
	}
	v.shown += (target - v.shown) * easing
	if math.Abs(target-v.shown) < 0.3 {
		v.shown = target
	}
	visible := time.Now().Before(v.visibleUntil)
	targetAlpha := 0.0
	if visible {
		targetAlpha = 100
	}
	rate := fadeOut
	if targetAlpha > v.alpha {
		rate = fadeIn
	}
	v.alpha += (targetAlpha - v.alpha) * rate
	if math.Abs(targetAlpha-v.alpha) < 1 {
		v.alpha = targetAlpha
	}
	active := visible || v.shown != target || v.alpha != 0
	v.mu.Unlock()
	v.publish()
	return active
}

func (v *volume) publish() {
	v.mu.Lock()
	text := strconv.Itoa(v.value)
	if v.muted {
		text = "MUTE"
	}
	shown, alpha := v.shown, v.alpha
	v.mu.Unlock()
	v.vars.set("volume_anim", fmt.Sprintf("%.1f", shown))
	v.vars.set("volume", text)
	v.vars.set("volume_alpha", fmt.Sprintf("%.0f", alpha))
}

func (v *volume) setLocked(value int) error {
	v.value = max(0, min(100, value))
	v.revealLocked()
	if err := writeVolume(v.value); err != nil {
		return err
	}
	if v.muted && value > 0 {
		v.muted = false
		return writeMute(false)
	}
	return nil
}

func (v *volume) step(direction int) error {
	v.mu.Lock()
	defer v.mu.Unlock()
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
	return v.setLocked(v.value + direction*amount)
}

func (v *volume) drag(pixels float64) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.dragRemainder += pixels * dragPercentPerPixel
	amount := int(v.dragRemainder)
	if amount == 0 {
		return nil
	}
	v.dragRemainder -= float64(amount)
	return v.setLocked(v.value + amount)
}

func (v *volume) toggleMute() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.muted = !v.muted
	v.revealLocked()
	return writeMute(v.muted)
}
