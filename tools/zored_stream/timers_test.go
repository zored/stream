package main

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestTimers(t *testing.T) *timers {
	postVar = func(string, string) error { return nil }
	timerSound = func() error { return nil }
	tm, err := newTimers(&vars{sent: map[string]string{}}, filepath.Join(t.TempDir(), "timers.json"))
	if err != nil {
		t.Fatal(err)
	}
	tm.release = func() {}
	return tm
}

func tap(t *testing.T, tm *timers) {
	if err := tm.down(); err != nil {
		t.Fatal(err)
	}
	if err := tm.up(); err != nil {
		t.Fatal(err)
	}
}

func TestTimerKnobCaptureAndReturn(t *testing.T) {
	tm := newTestTimers(t)
	if tm.turn(1) {
		t.Fatal("idle turn must pass to seek")
	}
	tap(t, tm)
	if !tm.turn(1) || !tm.turn(1) {
		t.Fatal("edit turn must be taken")
	}
	tap(t, tm)
	if len(tm.items) != 1 || tm.turn(1) {
		t.Fatalf("second tap must start and release the knob: %v", tm.items)
	}
	tap(t, tm)
	tm.turn(1)
	tm.editTill = time.Now().Add(-time.Millisecond)
	tm.tick()
	if len(tm.items) != 2 || tm.turn(1) {
		t.Fatal("edit timeout must start and release the knob")
	}
	tap(t, tm)
	tm.turn(1)
	if took, _ := tm.knob(true); !took {
		t.Fatal("knob press while editing must be taken")
	}
	if took, err := tm.knob(false); !took || err != nil || len(tm.items) != 3 || tm.turn(1) {
		t.Fatal("knob press must start the timer and release the knob")
	}
	if took, _ := tm.knob(true); took {
		t.Fatal("idle knob press must pass to scroll")
	}
}

func TestTimerOrderPersistAndRing(t *testing.T) {
	tm := newTestTimers(t)
	now := time.Now()
	tm.items = []timerItem{{At: now.Add(time.Hour)}}
	tm.edit, tm.editTill = time.Minute, now.Add(time.Second)
	if err := tm.commitLocked(now); err != nil {
		t.Fatal(err)
	}
	if !tm.items[0].At.Before(tm.items[1].At) {
		t.Fatal("nearest first")
	}
	if s, _ := tm.line(now); s != "1m +1" {
		t.Errorf("line %q", s)
	}
	back, err := newTimers(tm.vars, tm.file)
	if err != nil || len(back.items) != 2 {
		t.Fatalf("reload: %v %v", back, err)
	}
	tm.items[0].At = now.Add(-time.Second)
	tm.tick()
	if len(tm.items) != 1 || !time.Now().Before(tm.ringTill) {
		t.Fatal("due timer must ring and drop")
	}
	tap(t, tm)
	if time.Now().Before(tm.ringTill) || tm.editing(time.Now()) {
		t.Fatal("tap while ringing must only silence")
	}
}

func TestAlarmFromZeroLeft(t *testing.T) {
	tm := newTestTimers(t)
	tap(t, tm)
	tm.turn(-1)
	if tm.alarm.IsZero() || !tm.alarm.After(time.Now()) {
		t.Fatalf("left from zero must switch to alarm, got %v", tm.alarm)
	}
}

func TestTileLayers(t *testing.T) {
	postVar = func(string, string) error { return nil }
	inCall, got := false, ""
	s := &slot{vars: &vars{sent: map[string]string{}}, name: "tile_24", layers: []layer{
		{name: "call", active: func() bool { return inCall }, press: func() error { got = "call"; return nil }},
		{name: "cal", press: func() error { got = "cal"; return nil }},
	}}
	s.press()
	inCall = true
	s.press()
	if got != "call" || s.vars.sent["tile_24"] != "call" {
		t.Fatalf("call must win: %s", got)
	}
}
