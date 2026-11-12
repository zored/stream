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

func TestAIHoldEndsPress(t *testing.T) {
	a := &aiSwitch{}
	a.down()
	a.hold()
	if a.holding() {
		t.Fatal("press still held after the hold fired")
	}
}

func TestAILostReleaseExpires(t *testing.T) {
	a := &aiSwitch{}
	a.down()
	a.heldAt = a.heldAt.Add(-2 * aiHeldMax)
	if a.holding() {
		t.Fatal("press without release never expired")
	}
}
