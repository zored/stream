package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeDesktop(t *testing.T, dir, id, body string) {
	t.Helper()
	p := filepath.Join(dir, "Claude", "claude-code-sessions", "acct", "org", id+".json")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDesktopScan(t *testing.T) {
	dir := t.TempDir()
	writeDesktop(t, dir, "local_a", `{"sessionId":"local_a","cliSessionId":"cli-a","lastFocusedAt":1,"lastTurnEdgeAt":2}`)
	writeDesktop(t, dir, "local_b", `{"sessionId":"local_b","lastFocusedAt":3,"lastTurnEdgeAt":2}`)
	writeDesktop(t, dir, "local_c", `{"sessionId":"local_c","isArchived":true,"lastTurnEdgeAt":2}`)
	writeDesktop(t, dir, "local_d", `{"sessionId":"local_d","scheduledTaskId":"x","lastTurnEdgeAt":2}`)
	writeDesktop(t, dir, "local_e", `{broken`)
	d := newDesktopSessions(dir)
	recs, err := d.scan()
	if err != nil {
		t.Fatal(err)
	}
	unread := map[string]bool{}
	for id, r := range recs {
		unread[id] = r.unread()
	}
	want := map[string]bool{"local_a": true, "local_b": false, "local_c": false, "local_d": false}
	if len(unread) != len(want) {
		t.Fatalf("got %v, want %v", unread, want)
	}
	for id, w := range want {
		if unread[id] != w {
			t.Errorf("%s unread = %v, want %v", id, unread[id], w)
		}
	}
	s := recs["local_a"].session()
	if s.ID != "cli-a" || s.Host != "local_a" || s.Kind != agentDone || !s.Since.Equal(time.UnixMilli(2)) {
		t.Errorf("session = %+v", s)
	}

	writeDesktop(t, dir, "local_a", `{"sessionId":"local_a","cliSessionId":"cli-a","lastFocusedAt":5,"lastTurnEdgeAt":2}`)
	if recs, err = d.scan(); err != nil {
		t.Fatal(err)
	}
	if recs["local_a"].unread() {
		t.Error("local_a still unread after focus")
	}
}
