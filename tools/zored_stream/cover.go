package main

import (
	"math"
	"strconv"
	"sync"
	"time"
)

const (
	glyphHold = 1500 * time.Millisecond
	pauseBlip = 600 * time.Millisecond
)

type glyph struct {
	mu           sync.Mutex
	playing      bool
	alpha        float64
	visibleUntil time.Time
	pausedAt     time.Time
	wake         chan struct{}
	vars         *vars
}

func newGlyph(v *vars) *glyph {
	return &glyph{alpha: 100, wake: make(chan struct{}, 1), vars: v}
}

func coverURI(t track) string {
	if t.Artwork == "" {
		return ""
	}
	return "data:image/jpeg;base64," + t.Artwork
}

func (g *glyph) setPlaying(playing bool) {
	g.mu.Lock()
	if g.playing == playing {
		g.mu.Unlock()
		return
	}
	g.playing = playing
	now := time.Now()
	if !playing {
		g.pausedAt = now
		g.mu.Unlock()
		time.AfterFunc(pauseBlip, g.wakeUp)
		return
	}
	blip := now.Sub(g.pausedAt) < pauseBlip
	g.mu.Unlock()
	if !blip {
		g.show()
	}
}

func (g *glyph) show() {
	g.mu.Lock()
	g.visibleUntil = time.Now().Add(glyphHold)
	g.mu.Unlock()
	g.wakeUp()
}

func (g *glyph) wakeUp() {
	select {
	case g.wake <- struct{}{}:
	default:
	}
}

func (g *glyph) animate() {
	for range g.wake {
		ticker := time.NewTicker(time.Second / animationHz)
		for range ticker.C {
			if !g.frame() {
				break
			}
		}
		ticker.Stop()
	}
}

func (g *glyph) frame() bool {
	g.mu.Lock()
	visible := !g.playing && time.Since(g.pausedAt) >= pauseBlip || time.Now().Before(g.visibleUntil)
	target := 0.0
	if visible {
		target = 100
	}
	rate := fadeOut
	if target > g.alpha {
		rate = fadeIn
	}
	g.alpha += (target - g.alpha) * rate
	if math.Abs(target-g.alpha) < 1 {
		g.alpha = target
	}
	active := g.alpha != target || (g.playing && visible)
	g.mu.Unlock()
	g.publish()
	return active
}

func (g *glyph) publish() {
	g.mu.Lock()
	alpha := g.alpha
	g.mu.Unlock()
	g.vars.set("np_glyph", strconv.FormatFloat(alpha, 'f', 0, 64))
}
