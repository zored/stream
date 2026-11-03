package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

type keyLog struct {
	mu     sync.Mutex
	events []string
}

func (k *keyLog) add(e string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.events = append(k.events, e)
	return nil
}

func (k *keyLog) get() []string {
	k.mu.Lock()
	defer k.mu.Unlock()
	return slices.Clone(k.events)
}

func fakeScroller(t *testing.T, tab bool) (*scroller, *keyLog) {
	k := &keyLog{}
	return &scroller{
		file:   filepath.Join(t.TempDir(), "scroll-mode"),
		tab:    tab,
		wheel:  func(v, h int) error { return k.add(fmt.Sprintf("wheel %d %d", v, h)) },
		modKey: func(m modifier, down bool) error { return k.add(fmt.Sprintf("%s %v", m, down)) },
		tabKey: func(m modifier, c chord, shift bool) error {
			return k.add(fmt.Sprintf("%s-%s %v", m, [...]string{"tab", "grave"}[c], shift))
		},
		now: time.Now,
	}, k
}

func TestScrollTabReleasesCtrl(t *testing.T) {
	s, k := fakeScroller(t, true)
	if err := s.turn(1); err != nil {
		t.Fatal(err)
	}
	if err := s.turn(-1); err != nil {
		t.Fatal(err)
	}
	want := []string{"ctrl true", "ctrl-tab false", "ctrl-tab true"}
	if got := k.get(); !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	time.Sleep(tabRelease + 200*time.Millisecond)
	want = append(want, "ctrl false")
	if got := k.get(); !slices.Equal(got, want) {
		t.Fatalf("after %v: got %v, want %v", tabRelease, got, want)
	}
}

func TestScrollPressReleasesCtrl(t *testing.T) {
	s, k := fakeScroller(t, true)
	if err := s.turn(1); err != nil {
		t.Fatal(err)
	}
	if err := s.hold(true); err != nil {
		t.Fatal(err)
	}
	if got := k.get(); !slices.Equal(got, []string{"ctrl true", "ctrl-tab false", "ctrl false"}) {
		t.Fatalf("got %v", got)
	}
	time.Sleep(tabRelease + 200*time.Millisecond)
	if got := len(k.get()); got != 3 {
		t.Fatalf("timer fired after the press released Ctrl: %v", k.get())
	}
}

func TestScrollSignalReleasesCtrl(t *testing.T) {
	s, k := fakeScroller(t, true)
	if err := s.turn(1); err != nil {
		t.Fatal(err)
	}
	s.release()
	s.release()
	if got := k.get(); !slices.Equal(got, []string{"ctrl true", "ctrl-tab false", "ctrl false"}) {
		t.Fatalf("got %v", got)
	}
}

func TestScrollModes(t *testing.T) {
	s, k := fakeScroller(t, false)
	if err := s.turn(1); err != nil {
		t.Fatal(err)
	}
	if err := s.hold(true); err != nil {
		t.Fatal(err)
	}
	if err := s.turn(1); err != nil {
		t.Fatal(err)
	}
	if err := s.hold(false); err != nil {
		t.Fatal(err)
	}
	if s.tab {
		t.Fatal("press + turn switched the mode")
	}
	if err := s.hold(true); err != nil {
		t.Fatal(err)
	}
	if err := s.hold(false); err != nil {
		t.Fatal(err)
	}
	if !s.tab {
		t.Fatal("click did not switch to tab mode")
	}
	if data, err := os.ReadFile(s.file); err != nil || string(data) != "tab\n" {
		t.Fatalf("mode not persisted: %q %v", data, err)
	}
	for _, e := range k.get() {
		if !strings.HasPrefix(e, "wheel") {
			t.Fatalf("wheel mode posted keys: %v", k.get())
		}
	}
}

func (m modifier) String() string {
	return [...]string{"none", "ctrl", "cmd"}[m]
}

func TestScrollTabPressHoldsCmd(t *testing.T) {
	s, k := fakeScroller(t, true)
	if err := s.turn(1); err != nil {
		t.Fatal(err)
	}
	if err := s.hold(true); err != nil {
		t.Fatal(err)
	}
	for _, d := range []int{1, -1} {
		if err := s.turn(d); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(tabRelease + 200*time.Millisecond)
	if err := s.hold(false); err != nil {
		t.Fatal(err)
	}
	want := []string{"ctrl true", "ctrl-tab false", "ctrl false", "cmd true", "cmd-tab false", "cmd-tab true", "cmd false"}
	if got := k.get(); !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if !s.tab {
		t.Fatal("press + turn switched the mode")
	}
}

func TestScrollCmdReleasedByTimerAndSignal(t *testing.T) {
	s, k := fakeScroller(t, true)
	if err := s.hold(true); err != nil {
		t.Fatal(err)
	}
	if err := s.turn(1); err != nil {
		t.Fatal(err)
	}
	s.release()
	s.release()
	want := []string{"cmd true", "cmd-tab false", "cmd false"}
	if got := k.get(); !slices.Equal(got, want) {
		t.Fatalf("signal: got %v, want %v", got, want)
	}
	if err := s.turn(1); err != nil {
		t.Fatal(err)
	}
	s.timer.Reset(10 * time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	want = append(want, "cmd true", "cmd-tab false", "cmd false")
	if got := k.get(); !slices.Equal(got, want) {
		t.Fatalf("timer: got %v, want %v", got, want)
	}
}

func TestScrollAppWindowsAfterSwitch(t *testing.T) {
	s, k := fakeScroller(t, true)
	clock := time.Now()
	s.now = func() time.Time { return clock }
	for _, step := range []func() error{
		func() error { return s.hold(true) },
		func() error { return s.turn(1) },
		func() error { return s.hold(false) },
		func() error { return s.turn(-1) },
		func() error { clock = clock.Add(appWindows - time.Second); return s.turn(1) },
		func() error { clock = clock.Add(appWindows + time.Second); return s.turn(1) },
	} {
		if err := step(); err != nil {
			t.Fatal(err)
		}
	}
	s.release()
	want := []string{"cmd true", "cmd-tab false", "cmd false", "cmd true", "cmd-grave true", "cmd-grave false", "cmd false", "ctrl true", "ctrl-tab false", "ctrl false"}
	if got := k.get(); !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
