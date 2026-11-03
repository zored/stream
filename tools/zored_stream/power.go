package main

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

const (
	sleepArmAfter    = time.Second
	sleepCancelAfter = 3 * time.Second
	awakeDefault     = time.Hour
	awakeStep        = 15 * time.Minute
	awakeMax         = 12 * time.Hour
	awakeEditFor     = 5 * time.Second
)

type power struct {
	mu       sync.Mutex
	vars     *vars
	awake    *awakeAssertion
	until    time.Time
	editTill time.Time
	accel    accel
	downAt   time.Time
	timers   []*time.Timer
}

func newPower(v *vars) *power {
	return &power{vars: v}
}

func (p *power) publish() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.publishLocked()
}

func (p *power) publishLocked() {
	held := time.Since(p.downAt)
	if !p.downAt.IsZero() && held >= sleepArmAfter && held < sleepCancelAfter {
		p.vars.set("power_svg", powerSVG("arm", ""))
		return
	}
	if p.awake == nil {
		p.vars.set("power_svg", powerSVG("off", ""))
		return
	}
	p.vars.set("power_svg", powerSVG("on", awakeLeft(time.Until(p.until), time.Now().Before(p.editTill))))
}

func (p *power) tick() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.awake != nil && !time.Now().Before(p.until) {
		if err := p.letSleep(); err != nil {
			log.Fatalf("keep-awake: %v", err)
		}
	}
	p.publishLocked()
}

func (p *power) down() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopTimers()
	p.downAt = time.Now()
	p.timers = []*time.Timer{
		time.AfterFunc(sleepArmAfter, p.publish),
		time.AfterFunc(sleepCancelAfter, p.publish),
	}
	return nil
}

func (p *power) up() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.downAt.IsZero() {
		return errors.New("release without press")
	}
	held := time.Since(p.downAt)
	p.downAt = time.Time{}
	p.stopTimers()
	defer p.publishLocked()
	switch {
	case held < sleepArmAfter:
		return p.toggle()
	case held < sleepCancelAfter:
		if err := p.letSleep(); err != nil {
			return err
		}
		p.vars.set("power_svg", powerSVG("off", ""))
		return sleepSystem()
	}
	return nil
}

func (p *power) turn(direction int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.awake == nil || !time.Now().Before(p.editTill) {
		return false
	}
	left := time.Until(p.until).Round(awakeStep) + time.Duration(p.accel.step(direction))*awakeStep
	p.until = time.Now().Add(max(awakeStep, min(awakeMax, left)))
	p.editTill = time.Now().Add(awakeEditFor)
	p.publishLocked()
	return true
}

func (p *power) toggle() error {
	if p.awake != nil {
		return p.letSleep()
	}
	a, err := holdAwake()
	if err != nil {
		return err
	}
	p.awake = &a
	p.until = time.Now().Add(awakeDefault)
	p.editTill = time.Now().Add(awakeEditFor)
	return nil
}

func (p *power) letSleep() error {
	if p.awake == nil {
		return nil
	}
	if err := p.awake.release(); err != nil {
		return err
	}
	p.awake = nil
	p.editTill = time.Time{}
	return nil
}

func (p *power) stopTimers() {
	for _, t := range p.timers {
		t.Stop()
	}
	p.timers = nil
}

func awakeLeft(left time.Duration, editing bool) string {
	minutes := int((left + time.Minute - 1) / time.Minute)
	fill := gray
	if editing {
		fill = "#fff"
	}
	return fmt.Sprintf(`<text x="50" y="91" text-anchor="middle" font-family="JetBrains Mono" font-size="22" fill="%s">%d:%02d</text>`,
		fill, minutes/60, minutes%60)
}

func powerSVG(state, below string) string {
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">`)
	cy := 50
	if below != "" {
		cy = 40
	}
	switch state {
	case "arm":
		b.WriteString(`<path d="M58 27a23 23 0 1 0 15 36 19 19 0 1 1-15-36z" fill="#fff"/>`)
	default:
		fill := "none"
		if state == "on" {
			fill = "#fff"
		}
		fmt.Fprintf(&b, `<g transform="translate(50 %d)" stroke="#fff" stroke-width="4" stroke-linecap="round">`, cy)
		fmt.Fprintf(&b, `<circle r="11" fill="%s"/>`, fill)
		for i := range 8 {
			fmt.Fprintf(&b, `<line y1="-18" y2="-26" transform="rotate(%d)"/>`, i*45)
		}
		b.WriteString(`</g>`)
	}
	b.WriteString(below)
	b.WriteString(`</svg>`)
	return dataURI(b.String())
}
