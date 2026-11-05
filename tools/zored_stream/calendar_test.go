package main

import (
	"strings"
	"testing"
	"time"
)

func TestCalKeepAndNext(t *testing.T) {
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.Local)
	items := calKeep([]calItem{
		{id: "past", title: "past", start: now.Add(-2 * time.Hour), end: now.Add(-time.Hour)},
		{id: "late", title: "late", start: now.Add(-time.Hour), end: now.Add(-time.Hour), reminder: true},
		{id: "far", title: "far", start: now.Add(13 * time.Hour), end: now.Add(14 * time.Hour)},
		{id: "day", title: "day", start: now.Add(-12 * time.Hour), end: now.Add(12 * time.Hour), allDay: true},
		{id: "soon", title: "soon", start: now.Add(3 * time.Minute), end: now.Add(time.Hour)},
		{id: "soon", title: "soon", start: now.Add(24 * time.Hour), end: now.Add(25 * time.Hour)},
	}, now)
	var got []string
	for _, it := range items {
		got = append(got, it.title)
	}
	if want := "day late soon far"; strings.Join(got, " ") != want {
		t.Fatalf("kept %v, want %s", got, want)
	}
	if i := calNext(items, now); items[i].title != "late" || calColor(items[i], now) != amberRGB {
		t.Errorf("next %d", i)
	}
	if calColor(items[2], now) != amberRGB || calColor(items[3], now) != 0 {
		t.Error("soon amber, far plain")
	}
	if calNext(items[2:3], now.Add(-13*time.Hour)) != -1 {
		t.Error("beyond 12 h must hide")
	}
}

func TestCalBrowse(t *testing.T) {
	c := &calendar{vars: &vars{sent: map[string]string{}}, release: func() {}, strip: func(string) {}, items: []calItem{{title: "a"}, {title: "b"}}}
	postVar = func(string, string) error { return nil }
	if c.turn(1) {
		t.Fatal("idle turn must pass to scroll")
	}
	if err := c.press(); err != nil || !c.browsing() {
		t.Fatalf("tap must browse: %v", err)
	}
	if !c.turn(1) || !c.turn(1) || c.pick != 1 {
		t.Errorf("pick %d, want clamped 1", c.pick)
	}
	if took, _ := c.knob(true); !took {
		t.Error("knob press in browse must be taken")
	}
}

func TestCalIn(t *testing.T) {
	for d, want := range map[time.Duration]string{40 * time.Minute: "+40m", 5*time.Hour + 10*time.Minute: "+5h", 26 * time.Hour: "+1d", -10 * time.Minute: "-10m"} {
		if got := calIn(d); got != want {
			t.Errorf("calIn(%v) = %s, want %s", d, got, want)
		}
	}
}

func TestCalWhenOngoing(t *testing.T) {
	now := time.Now()
	it := calItem{start: now.Add(-10 * time.Minute), end: now.Add(25*time.Minute + 30*time.Second)}
	if got := calWhen(it, now); got != "25m" {
		t.Errorf("ongoing = %q, want 25m", got)
	}
}

func TestCalNotes(t *testing.T) {
	got := calNotes("Ссылка: https://meet.example.com/abc?x=1\nобсудить (<https://a.b/c>) план www.x.ru/y\n---------------------------\nID звонка: 884942376\nКод доступа: 236435\n---------------------------")
	if want := "Ссылка: обсудить план"; got != want {
		t.Errorf("calNotes = %q, want %q", got, want)
	}
}

func TestCalNotesHTML(t *testing.T) {
	got := calNotes("план<br/>---------------------------<br>ID звонка: 1<br/><b>итоги</b> &amp; задачи<br />")
	if want := "план итоги & задачи"; got != want {
		t.Errorf("calNotes = %q, want %q", got, want)
	}
}

func TestCalLed(t *testing.T) {
	if calLed(false) != whiteRGB || calLed(true) != greenRGB {
		t.Error("calLed")
	}
}

func TestCalSoonBg(t *testing.T) {
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.Local)
	items := []calItem{
		{id: "r", start: now.Add(time.Minute), end: now.Add(time.Minute), reminder: true},
		{id: "m", start: now.Add(4 * time.Minute), end: now.Add(30 * time.Minute)},
	}
	if calBg(items, now.Add(-2*time.Minute), "") != 0 {
		t.Error("6 min ahead glows")
	}
	if calBg(items, now, "") != soonBgRGB || calBg(items, now.Add(20*time.Minute), "") != soonBgRGB {
		t.Error("no glow from 5 min ahead till the end")
	}
	if calBg(items, now.Add(30*time.Minute), "") != 0 {
		t.Error("glow after the end")
	}
	if g := calGlow(items, now); calBg(items, now.Add(10*time.Minute), g) != 0 {
		t.Error("dismissed glow came back")
	}
}
