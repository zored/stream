package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

const (
	timerEditFor   = 5 * time.Second
	timerHoldAfter = 400 * time.Millisecond
	timerRingFor   = time.Minute
	timerSoundGap  = 2 * time.Second
	timerTick      = 250 * time.Millisecond
	alarmStep      = 5 * time.Minute
)

var timerSteps = [...]time.Duration{time.Minute, 2 * time.Minute, 5 * time.Minute, 10 * time.Minute, 15 * time.Minute, 30 * time.Minute, time.Hour}

const (
	spinGap   = 200 * time.Millisecond
	spinShift = 4
)

type spin struct {
	last      time.Time
	direction int
	streak    int
}

func (s *spin) step(direction int, now time.Time) time.Duration {
	if direction == s.direction && now.Sub(s.last) <= spinGap {
		s.streak++
	} else {
		s.streak = 0
	}
	s.last, s.direction = now, direction
	return timerSteps[min(s.streak/spinShift, len(timerSteps)-1)]
}

var timerSound = func() error {
	return exec.Command("afplay", "/System/Library/Sounds/Glass.aiff").Start()
}

type timerItem struct {
	At    time.Time `json:"at"`
	Alarm bool      `json:"alarm,omitempty"`
}

type timers struct {
	mu       sync.Mutex
	frame    frame
	vars     *vars
	release  func()
	knobDown bool
	file     string
	items    []timerItem
	editTill time.Time
	edit     time.Duration
	alarm    time.Time
	spin     spin
	downAt   time.Time
	ringTill time.Time
	sounded  time.Time
}

func newTimers(v *vars, file string) (*timers, error) {
	t := &timers{vars: v, file: file}
	b, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return t, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &t.items); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	if len(t.items) > 1 {
		slices.SortFunc(t.items, func(a, b timerItem) int { return a.At.Compare(b.At) })
		t.items = t.items[:1]
	}
	return t, nil
}

func (t *timers) saveLocked() error {
	b, err := json.Marshal(t.items)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(t.file), 0o755); err != nil {
		return err
	}
	return os.WriteFile(t.file, b, 0o644)
}

func (t *timers) editing(now time.Time) bool {
	return now.Before(t.editTill)
}

func (t *timers) down() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.downAt = time.Now()
	return nil
}

func (t *timers) up() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.downAt.IsZero() {
		return errors.New("release without press")
	}
	now := time.Now()
	held := now.Sub(t.downAt) >= timerHoldAfter
	t.downAt = time.Time{}
	defer t.publishLocked(now)
	switch {
	case now.Before(t.ringTill):
		t.ringTill = time.Time{}
		return nil
	case t.editing(now):
		return t.commitLocked(now)
	case held:
		if len(t.items) == 0 {
			return nil
		}
		t.items = nil
		log.Print("timers: cancel")
		return t.saveLocked()
	}
	t.edit, t.alarm, t.editTill = 0, time.Time{}, now.Add(timerEditFor)
	go t.release()
	return nil
}

func (t *timers) knob(down bool) (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	if down {
		t.knobDown = t.editing(now)
		return t.knobDown, nil
	}
	if !t.knobDown {
		return false, nil
	}
	t.knobDown = false
	defer t.publishLocked(now)
	return true, t.commitLocked(now)
}

func (t *timers) capturing() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.editing(time.Now())
}

func (t *timers) turn(direction int) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	if !t.editing(now) {
		return false
	}
	step := t.spin.step(direction, now)
	switch {
	case !t.alarm.IsZero():
		step = max(step, alarmStep)
		t.alarm = t.alarm.Truncate(step).Add(time.Duration(direction) * step)
		if t.alarm.Before(now) {
			t.alarm = t.alarm.Add(24 * time.Hour)
		}
	case t.edit == 0 && direction < 0:
		t.alarm = now.Truncate(alarmStep).Add(alarmStep)
	default:
		t.edit = max(0, t.edit.Truncate(step)+time.Duration(direction)*step)
	}
	t.editTill = now.Add(timerEditFor)
	t.publishLocked(now)
	return true
}

func (t *timers) commitLocked(now time.Time) error {
	t.editTill = time.Time{}
	var it timerItem
	switch {
	case !t.alarm.IsZero():
		it = timerItem{At: t.alarm, Alarm: true}
	case t.edit > 0:
		it = timerItem{At: now.Add(t.edit)}
	default:
		return nil
	}
	t.items = []timerItem{it}
	log.Printf("timers: set %s", it.At.Format("15:04"))
	return t.saveLocked()
}

func (t *timers) tick() {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	if !t.editTill.IsZero() && !t.editing(now) {
		if err := t.commitLocked(now); err != nil {
			log.Print(err)
		}
	}
	due := 0
	for due < len(t.items) && !t.items[due].At.After(now) {
		due++
	}
	if due > 0 {
		t.items = t.items[due:]
		t.ringTill = now.Add(timerRingFor)
		log.Printf("timers: ring, %d due", due)
		if err := t.saveLocked(); err != nil {
			log.Print(err)
		}
	}
	if now.Before(t.ringTill) && now.Sub(t.sounded) >= timerSoundGap {
		t.sounded = now
		if err := timerSound(); err != nil {
			log.Printf("timers: sound: %v", err)
		}
	}
	t.publishLocked(now)
}

func (t *timers) publish() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.publishLocked(time.Now())
}

func (t *timers) publishLocked(now time.Time) {
	ring := "0"
	if now.Before(t.ringTill) {
		ring = "1"
	}
	t.vars.set("timer_ring", ring)
	text, color := t.line(now)
	t.vars.set("timer_svg", t.frame.render(text+"\n"+color, func() string { return timerSVG(text, color) }))
}

func (t *timers) line(now time.Time) (string, string) {
	if t.editing(now) {
		if !t.alarm.IsZero() {
			return t.alarm.Format("15:04"), "#fff"
		}
		if t.edit == 0 {
			return "", "#fff"
		}
		return timerLeft(t.edit), "#fff"
	}
	if len(t.items) == 0 {
		return calWeekdays[now.Weekday()], "#fff"
	}
	it := t.items[0]
	s := timerLeft(it.At.Sub(now))
	if it.Alarm {
		s = it.At.Format("15:04")
	}
	return s, gray
}

func timerLeft(d time.Duration) string {
	m := int((d + time.Minute - 1) / time.Minute)
	if m < 60 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dh%02d", m/60, m%60)
}

func timerSVG(text, fill string) string {
	if text == "" {
		return ""
	}
	return dataURI(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 88 24"><text x="44" y="19" text-anchor="middle" `+
		`font-family="%s" font-size="%g" fill="%s">%s</text></svg>`, stripFont, captionSize, fill, text))
}
