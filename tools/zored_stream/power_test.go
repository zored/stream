package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fakeAmphetamine(t *testing.T, running *bool) *[]string {
	var calls []string
	amphActive = func() (bool, error) { return *running, nil }
	amphStart = func() error { calls = append(calls, "start"); *running = true; return nil }
	amphEnd = func() error { calls = append(calls, "end"); *running = false; return nil }
	userActivity = func() error { calls = append(calls, "active"); return nil }
	systemSleep = func() error { calls = append(calls, "sleep"); return nil }
	postVar = func(string, string) error { return nil }
	screensOff = func() (bool, error) { return false, nil }
	locked = func() (bool, error) { return false, nil }
	t.Cleanup(func() {
		postVar, screensOff, locked = post, displaysAsleep, screenLocked
		amphActive, amphStart, amphEnd, userActivity, systemSleep = amphetamineActive, startAmphetamine, endAmphetamine, declareUserActivity, sleepSystem
	})
	return &calls
}

func tapPower(t *testing.T, p *power) {
	t.Helper()
	if err := p.down(); err != nil {
		t.Fatal(err)
	}
	if err := p.up(); err != nil {
		t.Fatal(err)
	}
}

func TestTapTogglesWithoutRestart(t *testing.T) {
	running := false
	calls := fakeAmphetamine(t, &running)
	p := newPower(&vars{sent: map[string]string{}})
	for range 3 {
		tapPower(t, p)
	}
	if got := strings.Join(*calls, ","); got != "start,end,active,start" {
		t.Errorf("calls = %s, want start,end,active,start", got)
	}
}

func TestTapEndsSessionStartedElsewhere(t *testing.T) {
	running := false
	calls := fakeAmphetamine(t, &running)
	p := newPower(&vars{sent: map[string]string{}})
	running = true
	tapPower(t, p)
	running = false
	tapPower(t, p)
	if got := strings.Join(*calls, ","); got != "end,active,start" {
		t.Errorf("calls = %s, want end,active,start: a tap on a stale tile must not restart a running session", got)
	}
}

func TestHoldSleeps(t *testing.T) {
	running := true
	calls := fakeAmphetamine(t, &running)
	p := newPower(&vars{sent: map[string]string{}})
	p.active = true
	p.downAt = time.Now().Add(-2 * time.Second)
	if err := p.up(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(*calls, ","); got != "end,sleep" {
		t.Errorf("calls = %s, want end,sleep", got)
	}
	*calls = nil
	p.downAt = time.Now().Add(-4 * time.Second)
	if err := p.up(); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 0 {
		t.Errorf("release past 3 s: calls = %v, want none", *calls)
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

func TestAmphetamineSuppressesScreensOff(t *testing.T) {
	sent := map[string]string{}
	postVar = func(name, value string) error { sent[name] = value; return nil }
	screensOff = func() (bool, error) { return true, nil }
	locked = func() (bool, error) { return false, nil }
	t.Cleanup(func() { postVar, screensOff, locked = post, displaysAsleep, screenLocked })
	p := newPower(&vars{sent: map[string]string{}})
	p.active = true
	p.publish()
	if sent["screen_off"] != "0" || sent["power_svg"] != powerSVG("on", false) {
		t.Errorf("active session, screens off: screen_off = %q, want 0 and the filled sun", sent["screen_off"])
	}
	p.active = false
	p.publish()
	if sent["screen_off"] != "1" || sent["power_svg"] != powerSVG("sleep", false) {
		t.Errorf("no session, screens off: screen_off = %q, want 1 and the moon", sent["screen_off"])
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
	if sent["screen_off"] != "1" || sent["power_svg"] != powerSVG("sleep", false) {
		t.Errorf("screen_off = %q, power_svg sleep = %v", sent["screen_off"], sent["power_svg"] == powerSVG("sleep", false))
	}
	screensOff = func() (bool, error) { return false, nil }
	p.publish()
	if sent["screen_off"] != "0" || sent["power_svg"] != powerSVG("off", true) {
		t.Errorf("locked after wake: screen_off = %q, want 0 and the sun with the lock beside it", sent["screen_off"])
	}
	locked = func() (bool, error) { return false, nil }
	p.publish()
	if sent["power_svg"] != powerSVG("off", false) {
		t.Error("unlocked: want the sun without the lock")
	}
}

func TestLockSitsBesideTheSun(t *testing.T) {
	for _, state := range []string{"on", "off"} {
		svg, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(powerSVG(state, true), "data:image/svg+xml;base64,"))
		if err != nil {
			t.Fatal(err)
		}
		sun := map[string]string{"on": "sun-fill", "off": "sun"}[state]
		if !strings.Contains(string(svg), iconAt(sun, "#fff", 50, iconCY, iconSize)) {
			t.Errorf("%s: want the sun kept under the lock", state)
		}
		if !strings.Contains(string(svg), iconAt("lock-fill", gray, lockCX, lockCY, lockSize)) {
			t.Errorf("%s: want a gray lock in the corner", state)
		}
	}
	if lockCX+lockSize/2 > 94 || lockCY-lockSize/2 < 6 {
		t.Errorf("lock leaves the 6%% inset: cx %g cy %g", lockCX, lockCY)
	}
}

func TestSleepMoonIsHollow(t *testing.T) {
	if powerSVG("sleep", false) != dataURI(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">`+iconAt("moon", "#fff", 50, 50, iconSize)+`</svg>`) {
		t.Error("screens-off moon: want the outline moon, centered")
	}
}

func TestDaemonNeverWritesClamshellSleep(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "kPMSetClamshellSleepState") {
			t.Errorf("%s: writes the shared clamshell-sleep bit; it clobbers powerd's desktop mode and sleeps a lid-closed Mac (invariants S8)", f)
		}
	}
}
