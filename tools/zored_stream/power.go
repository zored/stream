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
	awakeApplyAfter  = 500 * time.Millisecond
	awakePollEvery   = 2 * time.Second
	awakeRereadEvery = 30 * time.Second
	amphBridgeFor    = 5 * time.Second
)

var (
	amphStart    = startAmphetamine
	amphEnd      = endAmphetamine
	userActivity = declareUserActivity
	displayHold  = bridgeDisplay
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
	mu       sync.Mutex
	frame    frame
	vars     *vars
	session  amphSession
	readAt   time.Time
	until    time.Time
	editTill time.Time
	accel    accel
	apply    *time.Timer
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
	dark, err := screensOff()
	if err != nil {
		log.Print(err)
	}
	lock, err := locked()
	if err != nil {
		log.Print(err)
	}
	p.vars.set("screen_off", map[bool]string{true: "1", false: "0"}[dark])
	state, left := "on", ""
	switch {
	case !p.downAt.IsZero() && held >= sleepArmAfter && held < sleepCancelAfter:
		state = "arm"
	case dark:
		state = "sleep"
	case lock:
		state = "lock"
	case !p.session.active:
		state = "off"
	case p.session.timed:
		left = awakeLeft(max(0, time.Until(p.until)), time.Now().Before(p.editTill))
	}
	p.vars.set("power_svg", p.frame.render(state+"\n"+left, func() string { return powerSVG(state, left) }))
}

func (p *power) poll() {
	p.mu.Lock()
	busy := p.apply != nil || time.Now().Before(p.editTill)
	p.mu.Unlock()
	if busy {
		return
	}
	asserted, err := amphetamineAsserted()
	if err != nil {
		log.Print(err)
		return
	}
	p.mu.Lock()
	fresh := !p.readAt.IsZero() && asserted == p.session.active && (!asserted || time.Since(p.readAt) < awakeRereadEvery)
	p.mu.Unlock()
	if fresh {
		return
	}
	s, err := readAmphetamine()
	if err != nil {
		log.Print(err)
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.apply != nil || time.Now().Before(p.editTill) {
		return
	}
	p.session, p.readAt = s, time.Now()
	p.until = time.Now().Add(s.left)
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
		if !p.session.active {
			return p.start()
		}
		if err := displayHold(amphBridgeFor); err != nil {
			return err
		}
		if err := p.end(); err != nil {
			return err
		}
		return userActivity()
	case held < sleepCancelAfter:
		if err := p.end(); err != nil {
			return err
		}
		p.publishLocked()
		return systemSleep()
	}
	return nil
}

func (p *power) start() error {
	if err := restartAmphetamine(awakeDefault); err != nil {
		return err
	}
	p.session = amphSession{active: true, left: awakeDefault, timed: true}
	p.until = time.Now().Add(awakeDefault)
	p.editTill = time.Now().Add(awakeEditFor)
	return nil
}

func (p *power) end() error {
	if p.apply != nil {
		p.apply.Stop()
		p.apply = nil
	}
	p.editTill = time.Time{}
	if !p.session.active {
		return nil
	}
	if err := amphEnd(); err != nil {
		return err
	}
	p.session = amphSession{}
	return nil
}

func (p *power) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.editTill = time.Time{}
	p.publishLocked()
}

func (p *power) sizing() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.session.active && p.session.timed && time.Now().Before(p.editTill)
}

func (p *power) turn(direction int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.session.active || !p.session.timed || !time.Now().Before(p.editTill) {
		return false
	}
	left := time.Until(p.until).Round(awakeStep) + time.Duration(p.accel.step(direction))*awakeStep
	p.until = time.Now().Add(max(awakeStep, min(awakeMax, left)))
	p.editTill = time.Now().Add(awakeEditFor)
	if p.apply != nil {
		p.apply.Stop()
	}
	p.apply = time.AfterFunc(awakeApplyAfter, p.applyLength)
	p.publishLocked()
	return true
}

func (p *power) applyLength() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.apply = nil
	if !p.session.active {
		return
	}
	if err := restartAmphetamine(time.Until(p.until).Round(time.Minute)); err != nil {
		log.Print(err)
	}
}

// restartAmphetamine bridges the gap: Amphetamine drops the old display assertion before the new one, and with HID idle past displaysleep the screen goes dark in it.
func restartAmphetamine(d time.Duration) error {
	if err := displayHold(amphBridgeFor); err != nil {
		return err
	}
	return amphStart(d)
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
	return fmt.Sprintf(`<text x="50" y="%g" text-anchor="middle" font-family="%s" font-size="%g" fill="%s">%d:%02d</text>`,
		captionY, stripFont, captionSize, fill, minutes/60, minutes%60)
}

func powerSVG(state, below string) string {
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">`)
	switch state {
	case "sleep":
		b.WriteString(iconAt("moon-fill", "#fff", 50, 50, iconSize))
	case "lock":
		b.WriteString(iconAt("lock-fill", "#fff", 50, 50, iconSize))
	case "arm":
		b.WriteString(iconAt("moon-fill", "#fff", 50, iconCY, iconSize))
	case "on":
		b.WriteString(iconAt("sun-fill", "#fff", 50, iconCY, iconSize))
	default:
		b.WriteString(iconAt("sun", "#fff", 50, iconCY, iconSize))
	}
	b.WriteString(below)
	b.WriteString(`</svg>`)
	return dataURI(b.String())
}
