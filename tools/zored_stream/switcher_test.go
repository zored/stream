package main

import (
	"fmt"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"
)

type switchFixture struct {
	t       *testing.T
	w       *switcher
	v       *vars
	mu      sync.Mutex
	done    []string
	tiles   []string
	rows    [][]string
	windows map[int][]string
}

func newSwitchFixture(t *testing.T) *switchFixture {
	postVar = func(string, string) error { return nil }
	f := &switchFixture{t: t, v: &vars{sent: map[string]string{}}, windows: map[int][]string{1: {"x"}, 2: {"a", "b", ""}}}
	w := newSwitcher(f.v, newStripOverlay(f.v))
	f.w = w
	w.orient = func() (int, error) { return 0, nil }
	w.list = func() ([]switchApp, error) { return []switchApp{{1, "one"}, {2, "two"}, {3, "three"}}, nil }
	w.windows = func(pid int) ([]string, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		return slices.Clone(f.windows[pid]), nil
	}
	w.raise = func(pid, window int) error { f.log("raise %d %d", pid, window); return nil }
	w.closeWin = func(pid, window int) error {
		f.mu.Lock()
		f.windows[pid] = slices.Delete(f.windows[pid], window, window+1)
		f.mu.Unlock()
		f.log("close %d %d", pid, window)
		return nil
	}
	w.quit = func(pid int) error { f.log("quit %d", pid); return nil }
	w.render = func([]emojiGlyph, float64) (string, error) { return "strip", nil }
	w.listPNG = func(rows []string, offset float64) (string, error) {
		f.mu.Lock()
		f.rows = append(f.rows, rows)
		f.mu.Unlock()
		return fmt.Sprint(rows, offset), nil
	}
	w.bigPNG = func(pid int) (string, error) { f.tiles = append(f.tiles, strconv.Itoa(pid)); return "big", nil }
	w.badges = func() map[string]string { return map[string]string{"two": "3"} }
	w.frontPid = func() int { return 1 }
	w.idlePNG = func(apps []switchApp, badges []bool) (string, error) { return fmt.Sprint(badges), nil }
	go w.work()
	return f
}

func (f *switchFixture) log(format string, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.done = append(f.done, fmt.Sprintf(format, args...))
}

func (f *switchFixture) actions() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.done)
}

func (f *switchFixture) settle() {
	for range 100 {
		f.w.frame()
	}
	time.Sleep(20 * time.Millisecond)
	for range 10 {
		f.w.frame()
	}
}

func (f *switchFixture) want(actions ...string) {
	f.t.Helper()
	if got := f.actions(); !slices.Equal(got, actions) {
		f.t.Fatalf("actions %q, want %q", got, actions)
	}
}

func TestSwitcherIdle(t *testing.T) {
	f := newSwitchFixture(t)
	f.w.idle()
	f.w.idle()
	if got := f.v.sent["switch_tile"]; got != "[false true false]" {
		t.Fatalf("idle tile %q, want the badge on app two", got)
	}
}

func TestSwitcherScrollRaisesNothing(t *testing.T) {
	f := newSwitchFixture(t)
	if f.w.turn(emojiLeft, 1) {
		t.Fatal("closed switcher must not take the knob")
	}
	if err := f.w.tileDown(); err != nil {
		t.Fatal(err)
	}
	f.w.tileUp()
	f.settle()
	f.w.turn(emojiLeft, 1)
	f.settle()
	f.w.turn(emojiRight, 1)
	f.settle()
	f.want()
	if got := f.v.sent["emoji_r"]; got != "[a b two] 1" {
		t.Errorf("right strip %q, want the rows of app two at window 1", got)
	}
	if !slices.Equal(f.tiles, []string{"1", "2"}) {
		t.Errorf("tiles %q, want the big icons of apps 1 then 2", f.tiles)
	}
	f.w.start(emojiRight, 100)
	f.w.end(emojiRight)
	f.settle()
	f.want("raise 2 1")
	if !f.w.isOpen() {
		t.Error("a strip tap keeps the switcher open")
	}
	f.w.press(emojiLeft)
	f.w.release(emojiLeft)
	f.settle()
	f.want("raise 2 1", "raise 2 1")
	if f.w.isOpen() || f.v.sent["emoji_on"] != "0" {
		t.Error("a knob click must close the switcher")
	}
}

func TestSwitcherHold(t *testing.T) {
	f := newSwitchFixture(t)
	f.w.tileDown()
	f.w.tileUp()
	f.w.turn(emojiLeft, 1)
	f.settle()
	f.w.tileDown()
	time.Sleep(switchHoldClose + 100*time.Millisecond)
	f.settle()
	f.want("close 2 0")
	if got := f.v.sent["emoji_r"]; got != "[b two] 0" {
		t.Errorf("right strip %q, want the windows left after the close", got)
	}
	time.Sleep(switchHoldQuit)
	f.settle()
	f.want("close 2 0", "quit 2")
	f.w.tileUp()
	f.settle()
	if !f.w.isOpen() {
		t.Fatal("releasing a hold must keep the switcher open")
	}
	f.w.mu.Lock()
	apps := slices.Clone(f.w.apps)
	f.w.mu.Unlock()
	if !slices.Equal(apps, []switchApp{{1, "one"}, {3, "three"}}) {
		t.Errorf("apps %v, want the quit one gone", apps)
	}
	f.w.tileDown()
	f.w.tileUp()
	f.settle()
	f.want("close 2 0", "quit 2", "raise 3 -1")
	if f.w.isOpen() {
		t.Error("a tile tap must close the switcher")
	}
}
