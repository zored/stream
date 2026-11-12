package main

import "testing"

func TestAIHoldWithoutTurnFiresOnce(t *testing.T) {
	a := &aiSwitch{}
	if a.hold() {
		t.Fatal("hold without press fired")
	}
	a.down()
	if !a.hold() {
		t.Fatal("hold after press did not fire")
	}
	if a.hold() {
		t.Fatal("hold fired twice")
	}
	if !a.up() {
		t.Fatal("release after hold must be consumed")
	}
}

func TestAIHoldAfterTurnDoesNothing(t *testing.T) {
	a := &aiSwitch{}
	a.down()
	a.turned = true
	if a.hold() {
		t.Fatal("hold fired after a turn")
	}
}
