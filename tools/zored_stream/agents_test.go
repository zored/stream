package main

import (
	"slices"
	"testing"
	"time"
)

func TestAgentOrder(t *testing.T) {
	t0 := time.Unix(1000, 0)
	list := []*agentSession{
		{ID: "w", Kind: agentWorking, Since: t0},
		{ID: "d2", Kind: agentDone, Since: t0.Add(time.Minute)},
		{ID: "i2", Kind: agentInput, Since: t0.Add(time.Minute)},
		{ID: "d1", Kind: agentDone, Since: t0},
		{ID: "i1", Kind: agentInput, Since: t0},
	}
	slices.SortFunc(list, agentLess)
	var got []string
	for _, s := range list {
		got = append(got, s.ID)
	}
	if want := []string{"i1", "i2", "d1", "d2", "w"}; !slices.Equal(got, want) {
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
