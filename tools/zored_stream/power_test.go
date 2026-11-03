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
	if got := strings.Join(calls, ","); got != "start,end,active" {
		t.Errorf("calls = %s, want start,end,active", got)
	}
}
