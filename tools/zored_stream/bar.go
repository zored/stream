package main

import (
	"fmt"
	"math"
	"sync"
	"time"
)

const (
	visibleFor  = time.Second
	animationHz = 60
	easing      = 0.163
	fadeIn      = 0.368
	fadeOut     = 0.134
)

type bar struct {
	mu           sync.Mutex
	mode         string
	target       float64
	shown        float64
	alpha        float64
	visibleUntil time.Time
	wake         chan struct{}
	vars         *vars
}

func newBar(v *vars, mode string, value float64) *bar {
	return &bar{mode: mode, target: value, shown: value, wake: make(chan struct{}, 1), vars: v}
}

func (b *bar) show(mode string, value float64) {
	b.mu.Lock()
	if b.alpha == 0 && b.mode != mode {
		b.shown = value
	}
	b.mode, b.target = mode, value
	b.visibleUntil = time.Now().Add(visibleFor)
	b.mu.Unlock()
	select {
	case b.wake <- struct{}{}:
	default:
	}
}

func (b *bar) animate() {
	for range b.wake {
		ticker := time.NewTicker(time.Second / animationHz)
		for range ticker.C {
			if !b.frame() {
				break
			}
		}
		ticker.Stop()
	}
}

func (b *bar) frame() bool {
	b.mu.Lock()
	b.shown += (b.target - b.shown) * easing
	if math.Abs(b.target-b.shown) < 0.3 {
		b.shown = b.target
	}
	visible := time.Now().Before(b.visibleUntil)
	targetAlpha := 0.0
	if visible {
		targetAlpha = 100
	}
	rate := fadeOut
	if targetAlpha > b.alpha {
		rate = fadeIn
	}
	b.alpha += (targetAlpha - b.alpha) * rate
	if math.Abs(targetAlpha-b.alpha) < 1 {
		b.alpha = targetAlpha
	}
	active := visible || b.shown != b.target || b.alpha != 0
	b.mu.Unlock()
	b.publish()
	return active
}

func (b *bar) publish() {
	b.mu.Lock()
	mode, shown, alpha := b.mode, b.shown, b.alpha
	b.mu.Unlock()
	b.vars.set("bar_mode", mode)
	b.vars.set("bar_value", fmt.Sprintf("%.1f", shown))
	b.vars.set("bar_alpha", fmt.Sprintf("%.0f", alpha))
}

type accel struct {
	last      time.Time
	direction int
}

func (a *accel) step(direction int) int {
	now := time.Now()
	dt := time.Second
	if direction == a.direction {
		dt = now.Sub(a.last)
	}
	a.last, a.direction = now, direction
	switch {
	case dt > 200*time.Millisecond:
		return direction
	case dt > 100*time.Millisecond:
		return 2 * direction
	case dt > 50*time.Millisecond:
		return 3 * direction
	}
	return 4 * direction
}
