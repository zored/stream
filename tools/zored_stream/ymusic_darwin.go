package main

/*
#cgo LDFLAGS: -framework ApplicationServices -framework AppKit
#include <stdlib.h>
#include "ax_darwin.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
	"unsafe"
)

const (
	ymBundle      = "ru.yandex.desktop.music"
	ymRescanEvery = 2 * time.Second
	ymHotFor      = 3 * time.Second
	ymColdEvery   = 3 * time.Second
	ymPressGap    = 1500 * time.Millisecond
	ymTrackPrefix = "Трек "
	axMaxNodes    = 20000
)

type ymusic struct {
	mu        sync.Mutex
	pid       C.pid_t
	app       C.AXUIElementRef
	bar       C.AXUIElementRef
	lastScan  time.Time
	lastPress time.Time
	polled    time.Time
	hotTill   time.Time
	seenTitle string
	trusted   bool
	np        *nowPlaying
	vars      *vars
}

func newYmusic(np *nowPlaying, v *vars) *ymusic {
	return &ymusic{trusted: true, np: np, vars: v}
}

func find(root C.AXUIElementRef, kind C.int) C.AXUIElementRef {
	var truncated C.int
	found := C.ax_find(root, kind, axMaxNodes, &truncated)
	if truncated != 0 {
		log.Printf("ymusic: AX walk stopped at %d nodes", axMaxNodes)
	}
	return found
}

func (y *ymusic) dropBar() {
	if y.bar != 0 {
		C.ax_release(y.bar)
		y.bar = 0
	}
}

func (y *ymusic) connect() bool {
	bundle := C.CString(ymBundle)
	defer C.free(unsafe.Pointer(bundle))
	pid := C.ax_app_pid(bundle)
	if pid == 0 {
		y.pid = 0
		y.dropBar()
		if y.app != 0 {
			C.ax_release(y.app)
			y.app = 0
		}
		return false
	}
	if pid != y.pid {
		y.dropBar()
		if y.app != 0 {
			C.ax_release(y.app)
		}
		y.pid = pid
		y.app = C.ax_app(pid)
		y.lastScan = time.Time{}
		log.Printf("ymusic: found %s pid %d", ymBundle, pid)
	}
	return true
}

func (y *ymusic) playerBar() C.AXUIElementRef {
	if y.bar != 0 && C.ax_alive(y.bar) != 0 {
		return y.bar
	}
	y.dropBar()
	if y.app == 0 || time.Since(y.lastScan) < ymRescanEvery {
		return 0
	}
	y.lastScan = time.Now()
	y.bar = find(y.app, C.AX_BAR)
	if y.bar == 0 {
		log.Print("ymusic: player bar not found")
	}
	return y.bar
}

func value(element C.AXUIElementRef) (int, bool) {
	var v C.int
	if C.ax_int_value(element, &v) == 0 {
		return 0, false
	}
	return int(v), true
}

func (y *ymusic) stateLocked() (string, string) {
	if C.ax_trusted(0) == 0 {
		if y.trusted {
			log.Print("ymusic: no Accessibility permission: System Settings → Privacy & Security → Accessibility → zored_stream")
			C.ax_trusted(1)
		}
		y.trusted = false
		return "unknown", ""
	}
	y.trusted = true
	if !y.connect() {
		return "off", ""
	}
	bar := y.playerBar()
	if bar == 0 {
		return "unknown", ""
	}
	title := ""
	if link := findAttr(bar, "AXLink", "AXDescription", ymTrackPrefix, true); link != 0 {
		title = strings.TrimSpace(strings.TrimPrefix(axString(link, "AXDescription"), ymTrackPrefix))
		C.ax_release(link)
	}
	like := find(bar, C.AX_LIKE)
	if like == 0 {
		return "unknown", ""
	}
	defer C.ax_release(like)
	liked, ok := value(like)
	if !ok {
		return "unknown", ""
	}
	if liked == 1 {
		return "liked", title
	}
	if dislike := find(bar, C.AX_DISLIKE); dislike != 0 {
		defer C.ax_release(dislike)
		if v, ok := value(dislike); ok && v == 1 {
			return "disliked", title
		}
	}
	return "none", title
}

func (y *ymusic) poll() {
	title := y.np.snapshot().Title
	y.mu.Lock()
	now := time.Now()
	if title != y.seenTitle {
		y.seenTitle, y.hotTill = title, now.Add(ymHotFor)
	}
	hot := now.Before(y.hotTill) || now.Sub(y.lastPress) < ymHotFor || (y.pid != 0 && frontPid() == y.pid)
	if !hot && now.Sub(y.polled) < ymColdEvery {
		y.mu.Unlock()
		return
	}
	y.polled = now
	state, title := y.stateLocked()
	y.mu.Unlock()
	y.vars.set("ym_like", state)
	y.np.setLike(title, state)
}

func (y *ymusic) press(like bool) error {
	y.mu.Lock()
	defer y.mu.Unlock()
	if app := y.np.app(); app != ymBundle {
		return fmt.Errorf("Now Playing is %q, not %s", app, ymBundle)
	}
	if time.Since(y.lastPress) < ymPressGap {
		return errors.New("previous press is still in flight")
	}
	if !y.connect() {
		return errors.New("app is not running")
	}
	bar := y.playerBar()
	if bar == 0 {
		return errors.New("no player bar")
	}
	kind, name := C.int(C.AX_LIKE), "like"
	if !like {
		kind, name = C.AX_DISLIKE, "dislike"
	}
	target := find(bar, kind)
	if target == 0 {
		return fmt.Errorf("no %s button in the player bar", name)
	}
	defer C.ax_release(target)
	if err := C.ax_press(target); err != 0 {
		return fmt.Errorf("AXPress failed: %d", err)
	}
	y.lastPress = time.Now()
	log.Printf("ymusic: %s pressed", name)
	return nil
}

func (y *ymusic) pressAndPoll(like bool) func() error {
	return func() error {
		if err := y.press(like); err != nil {
			return err
		}
		y.poll()
		return nil
	}
}
