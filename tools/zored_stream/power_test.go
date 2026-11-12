package main

import (
	"strings"
	"testing"
	"time"
)

func TestAwakeLeft(t *testing.T) {
	for left, want := range map[time.Duration]string{
		time.Hour:                                ">1:00<",
		time.Hour + 41*time.Minute + time.Second: ">1:42<",
		30 * time.Second:                         ">0:01<",
	} {
		if got := awakeLeft(left, false); !strings.Contains(got, want) {
			t.Errorf("awakeLeft(%v) = %s, want %s", left, got, want)
		}
	}
}

func TestTapOffWakesDisplayWithoutSleep(t *testing.T) {
	var calls []string
	amphStart = func(time.Duration) error { calls = append(calls, "start"); return nil }
	amphEnd = func() error { calls = append(calls, "end"); return nil }
	userActivity = func() error { calls = append(calls, "active"); return nil }
	systemSleep = func() error { calls = append(calls, "sleep"); return nil }
	displayHold = func(time.Duration) error { calls = append(calls, "hold"); return nil }
	postVar = func(string, string) error { return nil }
	screensOff = func() (bool, error) { return false, nil }
	locked = func() (bool, error) { return false, nil }
	t.Cleanup(func() {
		postVar, screensOff, locked = post, displaysAsleep, screenLocked
		amphStart, amphEnd, userActivity, systemSleep, displayHold = startAmphetamine, endAmphetamine, declareUserActivity, sleepSystem, bridgeDisplay
	})
	p := newPower(&vars{sent: map[string]string{}})
	for range 2 {
		if err := p.down(); err != nil {
			t.Fatal(err)
		}
		if err := p.up(); err != nil {
			t.Fatal(err)
		}
	}
	if got := strings.Join(calls, ","); got != "hold,start,hold,end,active" {
		t.Errorf("calls = %s, want hold,start,hold,end,active", got)
	}
}

func TestTouchRateLimited(t *testing.T) {
	calls := 0
	userActivity = func() error { calls++; return nil }
	defer func() { userActivity = declareUserActivity }()
	a := &activity{}
	a.touch()
	a.touch()
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
	a.last = time.Now().Add(-touchEvery)
	a.touch()
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestScreensOffMoonThenLock(t *testing.T) {
	sent := map[string]string{}
	postVar = func(name, value string) error { sent[name] = value; return nil }
	screensOff = func() (bool, error) { return true, nil }
	locked = func() (bool, error) { return true, nil }
	t.Cleanup(func() { postVar, screensOff, locked = post, displaysAsleep, screenLocked })
	p := newPower(&vars{sent: map[string]string{}})
	p.publish()
	if sent["screen_off"] != "1" || sent["power_svg"] != powerSVG("sleep", "") {
		t.Errorf("screen_off = %q, power_svg sleep = %v", sent["screen_off"], sent["power_svg"] == powerSVG("sleep", ""))
	}
	screensOff = func() (bool, error) { return false, nil }
	p.publish()
	if sent["screen_off"] != "0" || sent["power_svg"] != powerSVG("lock", "") {
		t.Errorf("locked after wake: screen_off = %q", sent["screen_off"])
	}
	locked = func() (bool, error) { return false, nil }
	p.publish()
	if sent["power_svg"] != powerSVG("off", "") {
		t.Error("unlocked: want the sun")
	}
}
