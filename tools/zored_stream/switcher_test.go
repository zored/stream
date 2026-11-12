package main

import (
	"slices"
	"testing"
)

func TestSwitcher(t *testing.T) {
	postVar = func(string, string) error { return nil }
	v := &vars{sent: map[string]string{}}
	w := newSwitcher(v, newStripOverlay(v))
	w.orient = func() (int, error) { return 0, nil }
	w.list = func() ([]switchApp, error) { return []switchApp{{1, "one"}, {2, "two"}}, nil }
	w.windows = func(pid int) ([]string, error) { return []string{"a", "b", ""}, nil }
	raised := make(chan [2]int, 8)
	w.raise = func(pid, window int) error { raised <- [2]int{pid, window}; return nil }
	w.render = func(int, []emojiGlyph, float64) (string, error) { return "strip", nil }
	var titles []string
	w.title = func(text string) (string, error) { titles = append(titles, text); return text, nil }
	go w.work()
	settle := func() {
		for range 100 {
			w.frame()
		}
	}
	loaded := func() bool {
		w.mu.Lock()
		defer w.mu.Unlock()
		return w.loaded
	}
	wait := func() {
		for !loaded() {
		}
		settle()
	}
	if w.turn(emojiLeft, 1) {
		t.Fatal("closed switcher must not take the knob")
	}
	if err := w.pressTile(); err != nil {
		t.Fatal(err)
	}
	wait()
	if len(raised) != 0 {
		t.Fatal("opening must not raise anything")
	}
	if !w.turn(emojiLeft, 1) {
		t.Fatal("open switcher must take the knob")
	}
	settle()
	if got := <-raised; got != [2]int{2, -1} {
		t.Errorf("raised %v, want app 2", got)
	}
	wait()
	w.turn(emojiRight, 1)
	w.turn(emojiRight, 1)
	settle()
	if got := <-raised; got != [2]int{2, 2} {
		t.Errorf("raised %v, want window 2 of app 2", got)
	}
	if want := []string{"two", "a", "two"}; len(titles) < 3 || !slices.Equal(titles[len(titles)-3:], want) {
		t.Errorf("titles %q, want them to end with %q", titles, want)
	}
	if v.sent["emoji_on"] != "1" {
		t.Error("strip overlay must be on while open")
	}
	if err := w.pressTile(); err != nil {
		t.Fatal(err)
	}
	if v.sent["emoji_on"] != "0" || v.sent["switch_tile"] != switchIdle {
		t.Error("a tap on the open tile must close the switcher")
	}
}
