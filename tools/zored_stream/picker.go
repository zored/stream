package main

import (
	"math"
	"sync"
	"time"
)

type stripPicker interface {
	turn(side, direction int) bool
	press(side int) (bool, error)
	release(side int) bool
	start(side int, y float64)
	slide(side int, y float64) bool
	drag(side int, dy float64) bool
	end(side int) error
}

// pickers routes strip and knob input to the open picker; modes.go keeps at most one open.
type pickers []stripPicker

func (ps pickers) turn(side, direction int) bool {
	for _, p := range ps {
		if p.turn(side, direction) {
			return true
		}
	}
	return false
}

func (ps pickers) press(side int) (bool, error) {
	for _, p := range ps {
		if took, err := p.press(side); took {
			return true, err
		}
	}
	return false, nil
}

func (ps pickers) release(side int) bool {
	took := false
	for _, p := range ps {
		took = p.release(side) || took
	}
	return took
}

func (ps pickers) start(side int, y float64) {
	for _, p := range ps {
		p.start(side, y)
	}
}

func (ps pickers) slide(side int, y float64) bool {
	for _, p := range ps {
		if p.slide(side, y) {
			return true
		}
	}
	return false
}

func (ps pickers) drag(side int, dy float64) bool {
	for _, p := range ps {
		if p.drag(side, dy) {
			return true
		}
	}
	return false
}

func (ps pickers) end(side int) error {
	for _, p := range ps {
		if err := p.end(side); err != nil {
			return err
		}
	}
	return nil
}

// stripOverlay: the pickers share the strip layers (emoji_l, emoji_r), shown while any of them is open.
type stripOverlay struct {
	mu   sync.Mutex
	vars *vars
	on   map[string]bool
}

func newStripOverlay(v *vars) *stripOverlay {
	return &stripOverlay{vars: v, on: map[string]bool{}}
}

func (o *stripOverlay) show(name string, on bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.on[name] = on
	value := "0"
	for _, v := range o.on {
		if v {
			value = "1"
		}
	}
	o.vars.set("emoji_on", value)
}

func (s *emojiStrip) clamp(offset float64) float64 {
	return min(max(offset, 0), float64(max(len(s.list)-1, 0)))
}

func (s *emojiStrip) knob(items int) {
	s.vel = 0
	s.target = s.clamp(math.Round(s.target) + float64(items))
}

func (s *emojiStrip) begin(y float64) {
	s.touching, s.hasY, s.lastY, s.moved, s.vel = true, true, y, 0, 0
	s.lastMove = time.Now()
	s.target = s.offset
}

func (s *emojiStrip) dragBy(dy float64) {
	now := time.Now()
	items := dy / emojiPitch
	if s.mirror {
		items = -items
	}
	if dt := now.Sub(s.lastMove).Seconds(); dt > 0 && dt < 0.1 {
		s.vel = 0.5*s.vel + 0.5*items/dt
	} else {
		s.vel = 0
	}
	s.lastMove = now
	s.touching = true
	s.moved += math.Abs(dy)
	s.offset = s.clamp(s.offset + items)
	s.target = s.offset
}

func (s *emojiStrip) slideTo(y float64) {
	if s.hasY {
		s.dragBy(s.lastY - y)
	}
	s.hasY, s.lastY = true, y
}

// finish ends a touch: tap is a touch that barely moved, rows is the tapped item counted from the ring.
func (s *emojiStrip) finish() (tap bool, rows float64) {
	tap = s.moved < emojiTapPx
	y := s.lastY * stripDrawH / stripHeight
	if s.mirror {
		y = stripDrawH - y
	}
	s.touching, s.hasY = false, false
	if time.Since(s.lastMove) > 100*time.Millisecond || math.Abs(s.vel) < emojiFling {
		s.vel = 0
	}
	s.target = math.Round(s.offset)
	return tap, math.Round((y - emojiTop) / emojiPitch)
}

func (s *emojiStrip) step() {
	if s.touching {
		return
	}
	if s.vel != 0 {
		s.offset += s.vel / animationHz
		s.vel *= emojiFriction
		clamped := s.clamp(s.offset)
		if clamped != s.offset || math.Abs(s.vel) < emojiFling {
			s.offset, s.vel = clamped, 0
			s.target = math.Round(s.offset)
		}
		return
	}
	s.offset += (s.target - s.offset) * emojiSnap
	if math.Abs(s.target-s.offset) < 0.01 {
		s.offset = s.target
	}
}

func (s *emojiStrip) settled() (int, bool) {
	return int(math.Round(s.target)), !s.touching && s.vel == 0
}
