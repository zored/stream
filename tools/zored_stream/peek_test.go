package main

import (
	"errors"
	"testing"
	"time"
)

func TestPeek(t *testing.T) {
	c := &call{mic: "on", cam: "off", changed: make(chan struct{}, 1)}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(c.peekDown())
	must(c.peekUp())
	if c.inCall() {
		t.Fatal("tap: want hidden")
	}
	if err := c.micDown(); !errors.Is(err, errNoCall) {
		t.Fatalf("hidden micDown: %v", err)
	}
	if err := c.camDown(); !errors.Is(err, errNoCall) {
		t.Fatalf("hidden camDown: %v", err)
	}
	must(c.peekDown())
	if !c.inCall() {
		t.Fatal("hold over hidden: want shown")
	}
	c.peekAt = c.peekAt.Add(-holdMin - time.Millisecond)
	must(c.peekUp())
	if c.inCall() {
		t.Fatal("after hold: want hidden again")
	}
	c.setLocked(none, none)
	c.setLocked("on", "off")
	if !c.inCall() {
		t.Fatal("new call: want shown")
	}
}
