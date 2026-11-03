package main

import "testing"

func TestEmojiPicker(t *testing.T) {
	var sent []string
	e := newEmoji(&vars{sent: map[string]string{}})
	e.recents = func() ([]string, error) { return []string{"🔥"}, nil }
	e.send = func(s string) error { sent = append(sent, s); return nil }
	e.render = func([]emojiGlyph, bool) (string, error) { return "", nil }
	postVar = func(string, string) error { return nil }
	if e.turn(emojiLeft, 1) {
		t.Fatal("closed picker must not take the knob")
	}
	if err := e.tile(); err != nil {
		t.Fatal(err)
	}
	if got := e.top(emojiLeft); got != "🔥" {
		t.Errorf("left top = %q, want the recent", got)
	}
	if !e.turn(emojiRight, 1) {
		t.Fatal("open picker must take the knob")
	}
	for range 100 {
		e.frame()
	}
	if err := e.tile(); err != nil {
		t.Fatal(err)
	}
	if want := emojiGroups[0][0]; len(sent) != 1 || sent[0] != want {
		t.Errorf("sent %q, want %q", sent, want)
	}
}

func TestEmojiColumn(t *testing.T) {
	g := emojiColumn([]string{"a", "b", "c"}, 0.6)
	if len(g) != 3 || g[1].text != "b" {
		t.Errorf("column = %+v", g)
	}
}
