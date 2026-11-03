package main

import (
	"strings"
	"testing"
	"time"
)

func TestCalKeepAndNext(t *testing.T) {
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.Local)
	items := calKeep([]calItem{
		{title: "past", start: now.Add(-2 * time.Hour), end: now.Add(-time.Hour)},
		{title: "late", start: now.Add(-time.Hour), end: now.Add(-time.Hour), reminder: true},
		{title: "far", start: now.Add(13 * time.Hour), end: now.Add(14 * time.Hour)},
		{title: "day", start: now.Add(-12 * time.Hour), end: now.Add(12 * time.Hour), allDay: true},
		{title: "soon", start: now.Add(3 * time.Minute), end: now.Add(time.Hour)},
	}, now)
	var got []string
	for _, it := range items {
		got = append(got, it.title)
	}
	if want := "day late soon far"; strings.Join(got, " ") != want {
		t.Fatalf("kept %v, want %s", got, want)
	}
	if i := calNext(items, now); items[i].title != "late" || calColor(items[i], now) != amber {
		t.Errorf("next %d", i)
	}
	if calColor(items[2], now) != amber || calColor(items[3], now) != "" {
		t.Error("soon amber, far plain")
	}
	if calNext(items[2:3], now.Add(-13*time.Hour)) != -1 {
		t.Error("beyond 12 h must hide")
	}
}

func TestCalBrowse(t *testing.T) {
	c := &calendar{vars: &vars{sent: map[string]string{}}, release: func() {}, items: []calItem{{title: "a"}, {title: "b"}}}
	postVar = func(string, string) error { return nil }
	if c.turn(1) {
		t.Fatal("idle turn must pass to scroll")
	}
	c.down()
	if err := c.up(); err != nil || !c.browsing() {
		t.Fatalf("tap must browse: %v", err)
	}
	if !c.turn(1) || !c.turn(1) || c.pick != 1 {
		t.Errorf("pick %d, want clamped 1", c.pick)
	}
	if took, _ := c.knob(true); !took {
		t.Error("knob press in browse must be taken")
	}
}
