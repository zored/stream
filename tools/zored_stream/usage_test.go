package main

import (
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

func TestTimeLeft(t *testing.T) {
	for d, want := range map[time.Duration]string{102 * time.Minute: "1:42", 2 * time.Hour: "2:00", 59 * time.Minute: "0:59", -time.Minute: "0:00"} {
		if got := clockLeft(d); got != want {
			t.Errorf("clock %v: got %s want %s", d, got, want)
		}
	}
	for d, want := range map[time.Duration]string{95 * time.Hour: "3d", 24 * time.Hour: "1d", 5*time.Hour + 20*time.Minute: "5:20"} {
		if got := daysLeft(d); got != want {
			t.Errorf("days %v: got %s want %s", d, got, want)
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

func TestStripValue(t *testing.T) {
	now := at(19, 12)
	limits := []limit{lim("session", 30, "", ptr(now.Add(102*time.Minute))), lim("weekly_all", 23, "", ptr(at(23, 18)))}
	for name, c := range map[string]struct {
		mode stripMode
		text string
		pct  float64
	}{
		"session":   {stripMode{}, "1:42", 30},
		"switching": {stripMode{switching: true}, "day", 115},
		"week":      {stripMode{weekUntil: now.Add(time.Second)}, "4d", 23},
	} {
		if text, pct := stripValue(limits, limits[1], c.mode, now); text != c.text || pct != c.pct {
			t.Errorf("%s: got %q %v want %q %v", name, text, pct, c.text, c.pct)
		}
	}
}

func TestStripValueAfterReset(t *testing.T) {
	now := at(19, 12)
	week := lim("weekly_all", 23, "", ptr(at(23, 18)))
	if text, pct := stripValue([]limit{lim("session", 38, "", ptr(now.Add(-time.Minute))), week}, week, stripMode{}, now); text != "" || pct != 0 {
		t.Errorf("session reset passed: got %q %v", text, pct)
	}
	old := lim("weekly_all", 23, "", ptr(now.Add(-time.Hour)))
	if text, pct := stripValue([]limit{old}, old, stripMode{switching: true}, now); text != "" || pct != 0 {
		t.Errorf("week reset passed: got %q %v", text, pct)
	}
}
