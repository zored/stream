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
	postVar = func(string, string) error { return nil }
	t.Cleanup(func() {
		postVar = post
		amphStart, amphEnd, userActivity, systemSleep = startAmphetamine, endAmphetamine, declareUserActivity, sleepSystem
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
	if got := strings.Join(calls, ","); got != "active,start,end,active" {
		t.Errorf("calls = %s, want active,start,end,active", got)
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
