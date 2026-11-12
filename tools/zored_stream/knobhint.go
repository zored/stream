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
	hintInset     = 5
	hintEdgeY     = 10
	hintIdle      = 0.3
	hintLiveFor   = 700 * time.Millisecond
	hintFadeIn    = 0.25
	hintFadeOut   = 0.08
)

var hintCells = []string{"hint_l0", "hint_l2", "hint_r0", "hint_r1", "hint_r2"}

var hintWake = make(chan struct{}, 1)

func kickHints() {
	select {
	case hintWake <- struct{}{}:
	default:
	}
}

type knobHint struct {
	right  bool
	row    int
	owners []func() bool
	shown  bool
	flip   bool
	cell   string
	alpha  float64
	live   time.Time
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
	h.knobs["l0"] = &knobHint{row: 0}
	h.knobs["l2"] = &knobHint{row: 2}
	for row := range 3 {
		h.knobs[fmt.Sprintf("r%d", row)] = &knobHint{right: true, row: row}
	}
	for _, cell := range hintCells {
		v.set(cell, hintBlank)
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
		k.turned, k.live = true, time.Now().Add(hintLiveFor)
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
		k.target, k.downAt, k.turned, k.live = hintHoldScale, now, false, now.Add(hintLiveFor)
	default:
		k.target = 1
		if !k.downAt.IsZero() && !k.turned && now.Sub(k.downAt) < hintClickFor {
			k.grow += hintClickKick
		}
		k.downAt, k.live = time.Time{}, now.Add(hintLiveFor)
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
	moving, now := false, time.Now()
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
			flip := err == nil && stripFlipped(o, k.right)
			*k = knobHint{right: k.right, row: k.row, owners: k.owners, shown: true, flip: flip, cell: hintCell(k.right, k.row, flip), scale: 1, target: 1, alpha: hintIdle}
		case !active && k.shown:
			k.shown = false
			h.vars.set(k.cell, hintBlank)
		}
		if !k.shown {
			continue
		}
		moving = true
		k.spin += (-hintSpring*k.angle - hintDamping*k.spin) * dt
		k.angle = max(-hintSpinMax, min(hintSpinMax, k.angle+k.spin*dt))
		k.grow += (-hintSpring*(k.scale-k.target) - hintDamping*k.grow) * dt
		k.scale += k.grow * dt
		if !k.downAt.IsZero() || now.Before(k.live) {
			k.alpha += (1 - k.alpha) * hintFadeIn
		} else {
			k.alpha += (hintIdle - k.alpha) * hintFadeOut
		}
		h.vars.set(k.cell, hintSVG(k.right, k.flip, k.row, k.angle, k.scale, k.alpha))
	}
	return moving
}

var hintBlank = dataURI(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d"/>`, hintCellW, hintCellH))

// hintCell: a strip the fork turns 180° is drawn into mirrored slots, so row r shows next to knob 2 − r.
func hintCell(right bool, row int, flip bool) string {
	side := 'l'
	if right {
		side = 'r'
	}
	if flip {
		row = 2 - row
	}
	return fmt.Sprintf("hint_%c%d", side, row)
}

// hintSVG draws in the knob's own frame (the flip turn is undone by the fork): the top knob's chevron sits near the top, the bottom one's near the bottom.
func hintSVG(right, flip bool, row int, angle, scale, alpha float64) string {
	y := [3]int{hintEdgeY, hintCellH / 2, hintCellH - hintEdgeY}[row]
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
		`<g opacity="%.2f" transform="rotate(%d %d %d) translate(%g %d) rotate(%.1f) scale(%.2f %.2f)" fill="none" stroke-linecap="round" stroke-linejoin="round">`+
		`<path d="%s" stroke="#000" stroke-width="5.5"/><path d="%s" stroke="#fff" stroke-width="2.5"/></g></svg>`,
		hintCellW, hintCellH, alpha, turn, hintCellW/2, hintCellH/2, x, y, math.Round(angle*2)/2, scale*float64(mirror), scale, chevron, chevron))
}
