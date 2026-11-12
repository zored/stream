package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func writeDesktopConfig(t *testing.T, dir string, starred ...string) string {
	t.Helper()
	p := filepath.Join(dir, "Claude", "claude_desktop_config.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	b := `{"preferences":{"epitaxyPrefs":{"starred-local-code-sessions":[`
	for i, h := range starred {
		if i > 0 {
			b += ","
		}
		b += `"` + h + `"`
	}
	if err := os.WriteFile(p, []byte(b+`]}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Duration(len(starred)+1) * time.Second)
	if err := os.Chtimes(p, later, later); err != nil {
		t.Fatal(err)
	}
	return p
}

func testAgents(t *testing.T, starred ...string) (*agents, string) {
	t.Helper()
	old := postVar
	postVar = func(string, string) error { return nil }
	t.Cleanup(func() { postVar = old })
	dir := t.TempDir()
	cfg := writeDesktopConfig(t, dir, starred...)
	return newAgents(&vars{sent: map[string]string{}}, time.Hour, newDesktopSessions(dir), newPins(cfg)), dir
}

func TestAgentPinsFollowDesktop(t *testing.T) {
	dir := t.TempDir()
	p := newPins(writeDesktopConfig(t, dir, "local_a"))
	if err := p.refresh(); err != nil || !p.has("local_a") || p.has("local_b") {
		t.Fatalf("starred not read: %v %v", p.starred, err)
	}
	if !p.toggle("local_b") || p.toggle("local_a") {
		t.Fatal("toggle did not flip")
	}
	if err := p.refresh(); err != nil || !p.has("local_b") || p.has("local_a") {
		t.Fatal("pending toggles lost before Desktop wrote them")
	}
	writeDesktopConfig(t, dir, "local_b")
	if err := p.refresh(); err != nil || len(p.pending) != 0 || !p.has("local_b") {
		t.Fatalf("Desktop agreement did not settle pending: %v", p.pending)
	}
	p.pending["local_c"] = pinWant{true, time.Now().Add(-2 * pinPendingFor)}
	if p.refresh(); p.has("local_c") {
		t.Error("stale pending pin kept")
	}
}

func TestAgentPinnedStaysOnTile(t *testing.T) {
	a, dir := testAgents(t, "local_p", "local_z")
	writeDesktop(t, dir, "local_p", `{"sessionId":"local_p","cliSessionId":"cli-p","lastFocusedAt":5,"lastTurnEdgeAt":2,"spawnedFrom":{"sessionId":"local_root"}}`)
	writeDesktop(t, dir, "local_z", `{"sessionId":"local_z","isArchived":true,"lastFocusedAt":5,"lastTurnEdgeAt":2}`)
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
	a.pins.toggle("local_p")
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

func TestDesktopFamilyPreorder(t *testing.T) {
	dir := t.TempDir()
	writeDesktop(t, dir, "local_r", `{"sessionId":"local_r","lastActivityAt":1}`)
	writeDesktop(t, dir, "local_a", `{"sessionId":"local_a","lastActivityAt":2,"spawnedFrom":{"sessionId":"local_r"}}`)
	writeDesktop(t, dir, "local_b", `{"sessionId":"local_b","lastActivityAt":5,"spawnedFrom":{"sessionId":"local_r"}}`)
	writeDesktop(t, dir, "local_b1", `{"sessionId":"local_b1","lastActivityAt":3,"spawnedFrom":{"sessionId":"local_b"}}`)
	writeDesktop(t, dir, "local_x", `{"sessionId":"local_x","isArchived":true,"spawnedFrom":{"sessionId":"local_r"}}`)
	writeDesktop(t, dir, "local_o", `{"sessionId":"local_o"}`)
	d := newDesktopSessions(dir)
	if _, err := d.scan(); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range d.family("local_b1") {
		got = append(got, r.SessionID)
	}
	if want := []string{"local_r", "local_b", "local_b1", "local_a"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestAISlideSteps(t *testing.T) {
	for _, c := range []struct {
		anchor, y float64
		steps     int
		rest      float64
	}{
		{100, 90, 0, 100},
		{100, 100 - aiSlidePitch, 1, 100 - aiSlidePitch},
		{100, 100 + 2.5*aiSlidePitch, -2, 100 + 2*aiSlidePitch},
	} {
		if steps, rest := aiSlideSteps(c.anchor, c.y); steps != c.steps || rest != c.rest {
			t.Errorf("aiSlideSteps(%v, %v) = %d, %v; want %d, %v", c.anchor, c.y, steps, rest, c.steps, c.rest)
		}
	}
}
