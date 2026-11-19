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
	awakePollEvery   = 2 * time.Second
	awakeRereadEvery = 30 * time.Second
	lockSize         = 18.0
	lockCX           = 94 - lockSize/2
	lockCY           = 6 + lockSize/2
)

var (
	amphActive   = amphetamineActive
	amphStart    = startAmphetamine
	amphEnd      = endAmphetamine
	userActivity = declareUserActivity
	systemSleep  = sleepSystem
	screensOff   = displaysAsleep
	locked       = screenLocked
)

const touchEvery = 5 * time.Second

var touched = &activity{}

type activity struct {
	mu   sync.Mutex
	last time.Time
}

func (a *activity) touch() {
	a.mu.Lock()
	due := time.Since(a.last) >= touchEvery
	if due {
		a.last = time.Now()
	}
	a.mu.Unlock()
	if due {
		if err := userActivity(); err != nil {
			log.Print(err)
		}
	}
}

type power struct {
	mu     sync.Mutex
	frame  frame
	vars   *vars
	active bool
	readAt time.Time
	downAt time.Time
	timers []*time.Timer
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
	dark, err := screensOff()
	if err != nil {
		log.Print(err)
	}
	dark = dark && !p.active
	lock, err := locked()
	if err != nil {
		log.Print(err)
	}
	p.vars.set("screen_off", map[bool]string{true: "1", false: "0"}[dark])
	state := "on"
	switch {
	case !p.downAt.IsZero() && held >= sleepArmAfter && held < sleepCancelAfter:
		state = "arm"
	case dark:
		state = "sleep"
	case !p.active:
		state = "off"
	}
	lock = lock && (state == "on" || state == "off")
	p.vars.set("power_svg", p.frame.render(fmt.Sprint(state, lock), func() string { return powerSVG(state, lock) }))
}

func (p *power) poll() {
	asserted, err := amphetamineAsserted()
	if err != nil {
		log.Print(err)
		return
	}
	p.mu.Lock()
	fresh := !p.readAt.IsZero() && asserted == p.active && (!asserted || time.Since(p.readAt) < awakeRereadEvery)
	p.mu.Unlock()
	if fresh {
		return
	}
	active, err := amphActive()
	if err != nil {
		log.Print(err)
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.active, p.readAt = active, time.Now()
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
		if err := p.end(); err != nil {
			return err
		}
		p.publishLocked()
		return systemSleep()
	}
	return nil
}

// toggle asks Amphetamine first: "start new session" over a running one ends it and starts again, and every end can sleep a lid-closed Mac.
func (p *power) toggle() error {
	active, err := amphActive()
	if err != nil {
		return err
	}
	p.active, p.readAt = active, time.Now()
	if !active {
		if err := amphStart(); err != nil {
			return err
		}
		p.active = true
		return nil
	}
	if err := p.end(); err != nil {
		return err
	}
	return userActivity()
}

func (p *power) end() error {
	if !p.active {
		return nil
	}
	if err := amphEnd(); err != nil {
		return err
	}
	p.active = false
	return nil
}

func (p *power) stopTimers() {
	for _, t := range p.timers {
		t.Stop()
	}
	p.timers = nil
}

func powerSVG(state string, lock bool) string {
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">`)
	if lock {
		b.WriteString(iconAt("lock-fill", gray, lockCX, lockCY, lockSize))
	}
	switch state {
	case "sleep":
		b.WriteString(iconAt("moon", "#fff", 50, 50, iconSize))
	case "arm":
		b.WriteString(iconAt("moon-fill", "#fff", 50, iconCY, iconSize))
	case "on":
		b.WriteString(iconAt("sun-fill", "#fff", 50, iconCY, iconSize))
	default:
		b.WriteString(iconAt("sun", "#fff", 50, iconCY, iconSize))
	}
	b.WriteString(`</svg>`)
	return dataURI(b.String())
}
