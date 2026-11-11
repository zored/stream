package main

import "testing"

func TestEmojiPicker(t *testing.T) {
	var sent []string
	e := newEmoji(&vars{sent: map[string]string{}})
	e.recents = func() ([]string, error) { return []string{"🔥"}, nil }
	e.orient = func() (int, error) { return 90, nil }
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
	g := emojiColumn([]string{"a", "b", "c"}, 0.6, 90)
	if len(g) != 3 || g[1].text != "b" {
		t.Errorf("column = %+v", g)
	}
}

func TestUpright(t *testing.T) {
	want := map[[2]int]float64{{0, 0}: 0, {0, 1}: 0, {90, 0}: 90, {90, 1}: 270, {180, 0}: 0, {180, 1}: 0, {270, 0}: 90, {270, 1}: 270}
	for k, w := range want {
		if got := upright(k[0], k[1] == 1); got != w {
			t.Errorf("upright(%d, right=%d) = %v, want %v", k[0], k[1], got, w)
		}
	}
}
