package main

import (
	"slices"
	"testing"
	"time"
)

func TestAgentOrder(t *testing.T) {
	t0 := time.Unix(1000, 0)
	list := []*agentSession{
		{ID: "old", Kind: agentInput, Updated: t0},
		{ID: "kid-new", Kind: agentDone, Updated: t0.Add(3 * time.Minute), Child: true},
		{ID: "new", Kind: agentWorking, Updated: t0.Add(2 * time.Minute)},
		{ID: "kid-old", Kind: agentInput, Updated: t0, Child: true},
		{ID: "mid", Kind: agentRead, Updated: t0.Add(time.Minute)},
	}
	slices.SortFunc(list, agentLess)
	var got []string
	for _, s := range list {
		got = append(got, s.ID)
	}
	if want := []string{"new", "mid", "old", "kid-new", "kid-old"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDesktopDone(t *testing.T) {
	for _, c := range []struct {
		s    agentSession
		want bool
	}{
		{agentSession{Kind: agentDone, Host: "local_abc-1"}, true},
		{agentSession{Kind: agentInput, Host: "local_abc-1"}, false},
		{agentSession{Kind: agentDone}, false},
		{agentSession{Kind: agentDone, Host: "local_a;rm"}, false},
	} {
		if got := c.s.desktopDone(); got != c.want {
			t.Errorf("%+v: got %v want %v", c.s, got, c.want)
		}
	}
}

func TestAgentDotGrid(t *testing.T) {
	seen := map[[2]float64]bool{}
	for i := range agentMaxDots {
		x, y := agentDot(i, agentMaxDots)
		if x-11.5 < 5 || x+11.5 > 95 || y-11.5 < 5 || y+11.5 > 95 {
			t.Errorf("dot %d at %.1f,%.1f leaves the tile", i, x, y)
		}
		seen[[2]float64{x, y}] = true
	}
	if len(seen) != agentMaxDots {
		t.Errorf("got %d distinct dots, want %d", len(seen), agentMaxDots)
	}
	if x, y := agentDot(0, 1); x != 50 || y != 50 {
		t.Errorf("single dot at %.1f,%.1f", x, y)
	}
	if x, _ := agentDot(4, 5); x != 50 {
		t.Errorf("lone dot of row 2 at x=%.1f", x)
	}
}

func TestFrontDesktop(t *testing.T) {
	recs := map[string]desktopRecord{
		"local_a": {SessionID: "local_a", LastFocusedAt: 10},
		"local_b": {SessionID: "local_b", LastFocusedAt: 30, IsArchived: true},
		"local_c": {SessionID: "local_c", LastFocusedAt: 20},
	}
	if got := frontDesktop(recs); got != "local_c" {
		t.Errorf("got %q", got)
	}
	if got := frontDesktop(nil); got != "" {
		t.Errorf("empty: got %q", got)
	}
	if got := itermUID("w0t1p0:AB-12"); got != "AB-12" {
		t.Errorf("iterm uid %q", got)
	}
}

func TestAgentStep(t *testing.T) {
	for _, c := range []struct{ i, d, n, want int }{
		{-1, 1, 3, 0}, {-1, -1, 3, 2}, {0, 1, 3, 1}, {2, 1, 3, 0}, {0, -1, 3, 2}, {1, -2, 3, 2},
	} {
		if got := agentStep(c.i, c.d, c.n); got != c.want {
			t.Errorf("agentStep(%d,%d,%d) = %d, want %d", c.i, c.d, c.n, got, c.want)
		}
	}
}
