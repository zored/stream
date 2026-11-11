package main

import (
	"encoding/base64"
	"strings"
	"testing"
)

func hintTestVars() (*vars, map[string]string) {
	got := map[string]string{}
	postVar = func(name, value string) error { got[name] = value; return nil }
	return &vars{sent: map[string]string{}}, got
}

func decodeHint(t *testing.T, uri string) string {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(uri, "data:image/svg+xml;base64,"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestKnobHintShowsWhileOwnedAndSettles(t *testing.T) {
	defer func() { postVar = post }()
	v, got := hintTestVars()
	h := newKnobHints(v, func() (int, error) { return 0, nil })
	owned := false
	h.own("r0", func() bool { return owned })
	if h.step(1.0/60) || got["hint_r0"] != hintBlank {
		t.Fatal("hint shown without an owner")
	}
	owned = true
	h.step(1.0 / 60)
	if got["hint_r0"] == hintBlank || got["hint_r2"] != hintBlank {
		t.Fatal("owned knob not shown or the other one shown")
	}
	h.turn("r0", 1)
	peak := 0.0
	for range 60 {
		h.step(1.0 / 60)
		peak = max(peak, h.knobs["r0"].angle)
	}
	if peak < 10 || peak > hintSpinMax {
		t.Fatalf("clockwise turn peak angle %.1f", peak)
	}
	if a := h.knobs["r0"].angle; a > 1 || a < -1 {
		t.Fatalf("angle did not settle: %.1f", a)
	}
	h.press("r0", true)
	for range 60 {
		h.step(1.0 / 60)
	}
	if s := h.knobs["r0"].scale; s < hintHoldScale-0.05 {
		t.Fatalf("held scale %.2f", s)
	}
	h.press("r0", false)
	owned = false
	h.step(1.0 / 60)
	if got["hint_r0"] != hintBlank {
		t.Fatal("hint stays after the capture ended")
	}
}

func TestKnobHintClickPulses(t *testing.T) {
	defer func() { postVar = post }()
	v, _ := hintTestVars()
	h := newKnobHints(v, func() (int, error) { return 0, nil })
	h.own("r2", func() bool { return true })
	h.step(1.0 / 60)
	h.press("r2", true)
	h.press("r2", false)
	peak := 0.0
	for range 30 {
		h.step(1.0 / 60)
		peak = max(peak, h.knobs["r2"].scale)
	}
	if peak < 1.15 {
		t.Fatalf("click pulse peak %.2f", peak)
	}
}

func TestKnobHintSVGSideAndFlip(t *testing.T) {
	right := decodeHint(t, hintSVG(true, false, 0, 1, 0.3))
	if !strings.Contains(right, `opacity="0.30" transform="rotate(0 24 43) translate(44 43)`) || !strings.Contains(right, "scale(1.00 1.00)") {
		t.Fatal(right)
	}
	left := decodeHint(t, hintSVG(false, true, 12.3, 1.2, 1))
	if !strings.Contains(left, "rotate(180 24 43) translate(4 43) rotate(12.5) scale(-1.20 1.20)") {
		t.Fatal(left)
	}
}

func TestKnobHintCellFollowsFlippedStrip(t *testing.T) {
	defer func() { postVar = post }()
	v, got := hintTestVars()
	h := newKnobHints(v, func() (int, error) { return 90, nil })
	h.own("l2", func() bool { return true })
	h.own("r2", func() bool { return true })
	h.step(1.0 / 60)
	if got["hint_l0"] == hintBlank || got["hint_l2"] != hintBlank || got["hint_r2"] == hintBlank {
		t.Fatal("at 90° the left knob 3 hint must sit in the mirrored left cell 0, the right strip stays")
	}
	h.turn("l2", 1)
	for range 10 {
		h.step(1.0 / 60)
	}
	if a := h.knobs["l2"].alpha; a < 0.9 {
		t.Fatalf("alpha while turning %.2f", a)
	}
}
