package main

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"testing"
	"time"
)

func at(d, h int) time.Time { return time.Date(2026, 10, d, h, 0, 0, 0, time.Local) }

func lim(kind string, p float64, model string, reset *time.Time) limit {
	l := limit{Kind: kind, Percent: p, ResetsAt: reset}
	if model != "" {
		l.Scope = &limitScope{Model: &limitModel{model}}
	}
	return l
}

func ptr(t time.Time) *time.Time { return &t }

func TestDayPct(t *testing.T) {
	monday := ptr(at(26, 0))
	for name, c := range map[string]struct {
		pct   float64
		reset *time.Time
		now   time.Time
		want  float64
	}{
		"monday":               {7, monday, at(19, 12), 35},
		"tuesday":              {30, monday, at(20, 15), 75},
		"weekend":              {90, monday, at(24, 12), 90},
		"friday evening reset": {5, ptr(at(23, 18)), at(19, 12), 25},
		"no reset":             {30, nil, at(20, 12), 75},
	} {
		if got := dayPct(lim("weekly_all", c.pct, "", c.reset), c.now); got != c.want {
			t.Errorf("%s: got %v want %v", name, got, c.want)
		}
	}
	if got := workdayIndex(at(21, 10), at(27, 12)); got != 4 {
		t.Errorf("midday start +week: got %d", got)
	}
}

func TestLevel(t *testing.T) {
	for n, want := range map[float64]string{49: "low", 50: "normal", 80: "normal", 81: "high", 90: "high", 91: "critical"} {
		if got := levelOf(n); got != want {
			t.Errorf("%v: got %s want %s", n, got, want)
		}
	}
}

func TestScopedFor(t *testing.T) {
	limits := []limit{lim("weekly_all", 1, "", nil), lim("weekly_scoped", 2, "Fable", nil)}
	if l := scopedFor(limits, "Fable"); l == nil || l.Percent != 2 {
		t.Errorf("Fable: got %v", l)
	}
	if l := scopedFor(limits, "Sonnet"); l != nil {
		t.Errorf("Sonnet: got %v", l)
	}
}

func TestStripFrame(t *testing.T) {
	now := at(20, 15)
	limits := []limit{lim("session", 30, "", ptr(now.Add(time.Hour))), lim("weekly_all", 30, "", ptr(at(26, 0))), lim("weekly_scoped", 50, "Fable", ptr(at(26, 0)))}
	f := stripFrameFor(limits, stripLabel{model: "opus"}, now)
	if f.session != 30 || f.week != 30 || f.allowed != 40 || f.weekLvl != "normal" || f.failed {
		t.Errorf("opus: got %+v", f)
	}
	f = stripFrameFor(limits, stripLabel{model: "fable"}, now)
	if f.week != 50 || f.weekLvl != "over" || f.left5h != "1:00" || f.leftW != "5d 9h" {
		t.Errorf("fable: got %+v", f)
	}
	if f := stripFrameFor(nil, stripLabel{}, now); !f.failed {
		t.Errorf("no limits: got %+v", f)
	}
}

func TestWeekLevel(t *testing.T) {
	for n, want := range map[float64]string{49: "low", 80: "normal", 81: "critical", 100: "critical", 101: "over"} {
		if got := weekLevelOf(n); got != want {
			t.Errorf("%v: got %s want %s", n, got, want)
		}
	}
}

func TestTimeLeft(t *testing.T) {
	for d, want := range map[time.Duration]string{102 * time.Minute: "1:42", 59 * time.Minute: "0:59", -time.Minute: "0:00"} {
		if got := clockLeft(d); got != want {
			t.Errorf("clock %v: got %s want %s", d, got, want)
		}
	}
	for d, want := range map[time.Duration]string{29 * time.Hour: "1d 5h", 48 * time.Hour: "2d", 4*time.Hour + time.Minute: "5h", 40 * time.Minute: "40m"} {
		if got := daysLeft(d); got != want {
			t.Errorf("days %v: got %s want %s", d, got, want)
		}
	}
}

func TestStripFrameAfterReset(t *testing.T) {
	now := at(19, 12)
	limits := []limit{lim("session", 38, "", ptr(now.Add(-time.Minute))), lim("weekly_all", 23, "", ptr(now.Add(-time.Hour)))}
	if f := stripFrameFor(limits, stripLabel{}, now); f.session != 0 || f.week != 0 || f.allowed != 0 || f.weekLvl != "" {
		t.Errorf("resets passed: got %+v", f)
	}
}

func TestLimitsStale(t *testing.T) {
	now := at(19, 12)
	every := 15 * time.Minute
	for name, c := range map[string]struct {
		failed  bool
		fetched *time.Time
		want    bool
	}{
		"fresh":         {false, ptr(now.Add(-every)), false},
		"two periods":   {false, ptr(now.Add(-2 * every)), false},
		"old":           {false, ptr(now.Add(-2*every - time.Second)), true},
		"failed":        {true, ptr(now), true},
		"never fetched": {false, nil, true},
	} {
		if got := limitsStale(c.failed, c.fetched, now, every); got != c.want {
			t.Errorf("%s: got %v want %v", name, got, c.want)
		}
	}
}

func TestUsageWaitSurvivesSleep(t *testing.T) {
	now := time.Now()
	if w := usageWait(now.Add(time.Hour), now); w > usageTick {
		t.Fatalf("wait %v longer than tick %v: a sleeping Mac stalls the fetch", w, usageTick)
	}
	if w := usageWait(now.Add(-time.Minute), now); w > 0 {
		t.Fatalf("overdue fetch waits %v", w)
	}
}

func TestUsageRetryAfterOffline(t *testing.T) {
	now := time.Now()
	offline := fmt.Errorf("token refresh: %w", &url.Error{Op: "Post", URL: oauthTokenURL, Err: &net.OpError{
		Op: "proxyconnect", Net: "tcp", Err: &net.OpError{Op: "dial", Net: "tcp", Err: &net.DNSError{Err: "no such host", Name: "squid.ai-proxy.vk.team", IsNotFound: true}},
	}})
	for _, c := range []struct {
		name     string
		err      error
		failures int
		want     time.Duration
		counted  bool
	}{
		{"offline", offline, 5, usageOfflineRetry, false},
		{"keychain", fmt.Errorf("%w: no item", errKeychain), 5, usageKeychainRetry, false},
		{"server", errors.New("503 Service Unavailable"), 1, 30 * time.Minute, true},
		{"server capped", errors.New("503 Service Unavailable"), 9, usageMaxBackoff, true},
	} {
		next, counted := usageNextTry(c.err, c.failures, 15*time.Minute, 0, now)
		if next.Sub(now) != c.want || counted != c.counted {
			t.Errorf("%s: next in %v counted %v, want %v %v", c.name, next.Sub(now), counted, c.want, c.counted)
		}
	}
}
