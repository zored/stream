package main

import (
	"path/filepath"
	"testing"
	"time"
)

func testAgents(t *testing.T) (*agents, string) {
	t.Helper()
	old := postVar
	postVar = func(string, string) error { return nil }
	t.Cleanup(func() { postVar = old })
	dir := t.TempDir()
	pins, err := loadPins(filepath.Join(dir, "pins.json"))
	if err != nil {
		t.Fatal(err)
	}
	return newAgents(&vars{sent: map[string]string{}}, time.Hour, newDesktopSessions(dir), pins), dir
}

func TestAgentPinsPersist(t *testing.T) {
	file := filepath.Join(t.TempDir(), "sub", "pins.json")
	p, err := loadPins(file)
	if err != nil {
		t.Fatal(err)
	}
	if on, err := p.toggle("local_a"); err != nil || !on {
		t.Fatalf("toggle on = %v, %v", on, err)
	}
	if p, err = loadPins(file); err != nil || !p.has("local_a") {
		t.Fatalf("reloaded pins %v, %v", p, err)
	}
	if on, err := p.toggle("local_a"); err != nil || on {
		t.Fatalf("toggle off = %v, %v", on, err)
	}
	if p, _ = loadPins(file); p.has("local_a") {
		t.Error("unpinned chat still saved")
	}
}

func TestAgentPinnedStaysOnTile(t *testing.T) {
	a, dir := testAgents(t)
	writeDesktop(t, dir, "local_p", `{"sessionId":"local_p","cliSessionId":"cli-p","lastFocusedAt":5,"lastTurnEdgeAt":2}`)
	writeDesktop(t, dir, "local_z", `{"sessionId":"local_z","isArchived":true,"lastFocusedAt":5,"lastTurnEdgeAt":2}`)
	a.pins.set["local_p"], a.pins.set["local_z"] = true, true
	a.expire()
	s := a.sessions["cli-p"]
	if s == nil || s.Kind != agentRead || len(a.sessions) != 1 {
		t.Fatalf("pinned read chat not added alone: %v", a.sessions)
	}
	s.Since = time.Now().Add(-2 * agentReadFor)
	a.expire()
	if a.sessions["cli-p"] == nil {
		t.Fatal("pinned chat expired")
	}
	delete(a.pins.set, "local_p")
	a.expire()
	if a.sessions["cli-p"] != nil {
		t.Error("unpinned read chat kept past agentReadFor")
	}
}

func TestAgentClickPinsWhileBrowsing(t *testing.T) {
	a, dir := testAgents(t)
	writeDesktop(t, dir, "local_p", `{"sessionId":"local_p","title":"🎛 chat","lastFocusedAt":5,"lastTurnEdgeAt":2}`)
	if _, err := a.desktop.scan(); err != nil {
		t.Fatal(err)
	}
	s := &agentSession{ID: "cli-p", Kind: agentRead, Host: "local_p", Since: time.Now()}
	a.sessions[s.ID], a.cursor, a.browsedAt = s, s, time.Now()
	if err := a.click(); err != nil {
		t.Fatal(err)
	}
	if !a.pins.has("local_p") {
		t.Fatal("click while browsing did not pin")
	}
	if p := <-a.sidebar; p != (sidebarPin{"🎛 chat", true}) {
		t.Errorf("sidebar got %+v", p)
	}
	a.armClick()
	if err := a.armedClick(); err != nil {
		t.Fatal(err)
	}
	if a.pins.has("local_p") {
		t.Error("knob click while browsing did not unpin")
	}
	a.cursor = &agentSession{ID: "cli", Kind: agentDone}
	if err := a.togglePin(); err == nil {
		t.Error("pinned a chat that is not a Desktop one")
	}
}
