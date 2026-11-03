package main

import (
	"errors"
	"sync"
	"time"
)

const (
	sleepArmAfter    = time.Second
	sleepCancelAfter = 3 * time.Second
)

type power struct {
	mu     sync.Mutex
	vars   *vars
	awake  *awakeAssertion
	downAt time.Time
	timers []*time.Timer
}

func newPower(v *vars) *power {
	return &power{vars: v}
}

func (p *power) state() string {
	if p.awake != nil {
		return "on"
	}
	return "off"
}

func (p *power) publish() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.publishLocked()
}

func (p *power) publishLocked() {
	if !p.downAt.IsZero() && time.Since(p.downAt) >= sleepArmAfter && time.Since(p.downAt) < sleepCancelAfter {
		p.vars.set("power", "arm")
		return
	}
	p.vars.set("power", p.state())
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
		p.vars.set("power", "off")
		return sleepSystem()
	}
	return nil
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
	return nil
}

func (p *power) stopTimers() {
	for _, t := range p.timers {
		t.Stop()
	}
	p.timers = nil
}
