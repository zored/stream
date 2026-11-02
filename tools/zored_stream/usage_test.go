package main

import (
	"strings"
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

func linesText(t *testing.T, limits []limit, now time.Time) string {
	lines, err := usageLines(limits, now)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, l := range lines {
		var cells []string
		for _, c := range l.cells {
			cells = append(cells, c.text+":"+c.level)
		}
		text := strings.Join(cells, " ")
		if l.dim {
			text = "(" + text + ")"
		}
		out = append(out, text)
	}
	return strings.Join(out, " / ")
}

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

func TestLines(t *testing.T) {
	monday := ptr(at(26, 0))
	now := at(19, 12)
	limits := []limit{
		lim("session", 3, "", ptr(now.Add(170*time.Minute+24*time.Second))),
		lim("weekly_all", 23, "", monday),
		lim("weekly_scoped", 12, "Fable", monday),
	}
	for name, c := range map[string]struct {
		limits []limit
		want   string
	}{
		"all":              {limits, "(2h 51m:low) / 115:critical 23:low / (60:normal 12:low)"},
		"inactive session": {[]limit{lim("session", 0, "", nil), limits[1]}, "(5h:low) / 115:critical 23:low"},
		"no session":       {limits[1:2], "115:critical 23:low"},
	} {
		if got := linesText(t, c.limits, now); got != c.want {
			t.Errorf("%s: got %q want %q", name, got, c.want)
		}
	}
	if _, err := usageLines(limits[:1], now); err == nil {
		t.Error("no weekly: want error")
	}
}

func TestDurationLeft(t *testing.T) {
	for d, want := range map[time.Duration]string{65 * time.Minute: "1h 5m", 2 * time.Hour: "2h", 59 * time.Minute: "59m", -time.Minute: "0m"} {
		if got := durationLeft(d); got != want {
			t.Errorf("%v: got %s want %s", d, got, want)
		}
	}
}

func TestLevel(t *testing.T) {
	for n, want := range map[float64]string{49: "low", 50: "normal", 80: "normal", 81: "high", 90: "high", 91: "critical"} {
		if got := levelOf(n); got != want {
			t.Errorf("%v: got %s want %s", n, got, want)
		}
	}
}
