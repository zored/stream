package main

import (
	"fmt"
	"log"
	"math"
	"sync"
	"time"
)

const (
	hintSpring    = 360.0
	hintDamping   = 17.0
	hintSpin      = 650.0
	hintSpinMax   = 70.0
	hintHoldScale = 1.4
	hintClickKick = 9.0
	hintClickFor  = 400 * time.Millisecond
	hintCellW     = stripDrawW
	hintCellH     = 86
	hintInset     = 7
)

var hintWake = make(chan struct{}, 1)

func kickHints() {
	select {
	case hintWake <- struct{}{}:
	default:
	}
}

type knobHint struct {
	name   string
	right  bool
	owners []func() bool
	shown  bool
	flip   bool
	angle  float64
	spin   float64
	scale  float64
	grow   float64
	target float64
	downAt time.Time
	turned bool
}

type knobHints struct {
	mu     sync.Mutex
	vars   *vars
	knobs  map[string]*knobHint
	orient func() (int, error)
}

func newKnobHints(v *vars, orient func() (int, error)) *knobHints {
	h := &knobHints{vars: v, orient: orient, knobs: map[string]*knobHint{}}
	for _, k := range []*knobHint{{name: "hint_l2"}, {name: "hint_r0", right: true}, {name: "hint_r1", right: true}, {name: "hint_r2", right: true}} {
		h.knobs[k.name] = k
		v.set(k.name, hintBlank)
	}
	return h
}

func (h *knobHints) own(name string, owner func() bool) {
	k := h.knob(name)
	k.owners = append(k.owners, owner)
}

func (h *knobHints) knob(name string) *knobHint {
	k, ok := h.knobs[name]
	if !ok {
		log.Fatalf("knob hint: unknown %s", name)
	}
	return k
}

func (h *knobHints) turn(name string, direction int) {
	h.mu.Lock()
	k := h.knob(name)
	if k.shown {
		k.spin += float64(direction) * hintSpin
		k.turned = true
	}
	h.mu.Unlock()
	kickHints()
}

func (h *knobHints) press(name string, down bool) {
	h.mu.Lock()
	k := h.knob(name)
	now := time.Now()
	switch {
	case !k.shown:
	case down:
		k.target, k.downAt, k.turned = hintHoldScale, now, false
	default:
		k.target = 1
		if !k.downAt.IsZero() && !k.turned && now.Sub(k.downAt) < hintClickFor {
			k.grow += hintClickKick
		}
		k.downAt = time.Time{}
	}
	h.mu.Unlock()
	kickHints()
}

func (h *knobHints) run() {
	for range hintWake {
		ticker := time.NewTicker(time.Second / animationHz)
		for h.step(1.0 / animationHz) {
			<-ticker.C
		}
		ticker.Stop()
	}
}

func (h *knobHints) step(dt float64) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	moving := false
	for _, k := range h.knobs {
		active := false
		for _, owns := range k.owners {
			active = active || owns()
		}
		switch {
		case active && !k.shown:
			o, err := h.orient()
			if err != nil {
				log.Print(err)
			}
			*k = knobHint{name: k.name, right: k.right, owners: k.owners, shown: true, flip: err == nil && stripFlipped(o, k.right), scale: 1, target: 1}
		case !active && k.shown:
			k.shown = false
			h.vars.set(k.name, hintBlank)
		}
		if !k.shown {
			continue
		}
		moving = true
		k.spin += (-hintSpring*k.angle - hintDamping*k.spin) * dt
		k.angle = max(-hintSpinMax, min(hintSpinMax, k.angle+k.spin*dt))
		k.grow += (-hintSpring*(k.scale-k.target) - hintDamping*k.grow) * dt
		k.scale += k.grow * dt
		h.vars.set(k.name, hintSVG(k.right, k.flip, k.angle, k.scale))
	}
	return moving
}

var hintBlank = dataURI(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d"/>`, hintCellW, hintCellH))

func hintSVG(right, flip bool, angle, scale float64) string {
	x, mirror := float64(hintCellW-hintInset), 1
	if !right {
		x, mirror = hintInset, -1
	}
	turn := 0
	if flip {
		turn = 180
	}
	const chevron = `M-2.5 -6L3.5 0L-2.5 6`
	return dataURI(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d">`+
		`<g transform="rotate(%d %d %d) translate(%g %d) rotate(%.1f) scale(%.2f %.2f)" fill="none" stroke-linecap="round" stroke-linejoin="round">`+
		`<path d="%s" stroke="#000" stroke-width="5.5"/><path d="%s" stroke="#fff" stroke-width="2.5"/></g></svg>`,
		hintCellW, hintCellH, turn, hintCellW/2, hintCellH/2, x, hintCellH/2, math.Round(angle*2)/2, scale*float64(mirror), scale, chevron, chevron))
}
