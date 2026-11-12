package main

import (
	"cmp"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	switchOpenFor   = 4 * time.Second
	switchIconSize  = 30
	switchRecent    = 4
	switchIdleEvery = 2 * time.Second
	switchHoldClose = 400 * time.Millisecond
	switchHoldQuit  = 2 * time.Second
)

type switchApp struct {
	pid  int
	name string
}

type switchJobKind string

const (
	jobList   switchJobKind = "list"
	jobApp    switchJobKind = "activate"
	jobWindow switchJobKind = "raise"
	jobClose  switchJobKind = "close"
	jobQuit   switchJobKind = "quit"
)

type switchJob struct {
	kind   switchJobKind
	pid    int
	window int
	asked  time.Time
}

// switcher: the left strip picks an app, the right one a window of it; a tap, a knob click or a tile tap brings the pick to the front.
type switcher struct {
	mu       sync.Mutex
	vars     *vars
	overlay  *stripOverlay
	openTill time.Time
	strips   [2]*emojiStrip
	turned   bool
	apps     []switchApp
	app      int
	titles   []string
	loaded   bool
	want     [2]int
	tile     string
	front    int
	idleKey  string
	idleTile string
	recent   []switchApp
	knobDown [2]bool
	tileHeld bool
	held     bool
	holdGen  int
	wake     chan struct{}
	jobs     chan switchJob
	orient   func() (int, error)
	list     func() ([]switchApp, error)
	windows  func(pid int) ([]string, error)
	raise    func(pid, window int) error
	closeWin func(pid, window int) error
	quit     func(pid int) error
	render   func(glyphs []emojiGlyph, ringY float64) (string, error)
	listPNG  func(rows []string, offset float64) (string, error)
	bigPNG   func(pid int) (string, error)
	idlePNG  func(apps []switchApp, badges []bool) (string, error)
	badges   func() map[string]string
	frontPid func() int
}

var switchIdle = tileIcon("window-stack", "#fff")

func newSwitcher(v *vars, o *stripOverlay) *switcher {
	return &switcher{vars: v, overlay: o, wake: make(chan struct{}, 1), jobs: make(chan switchJob, 4), strips: [2]*emojiStrip{{}, {}},
		idleTile: switchIdle, orient: readOrientation, list: switchApps, windows: switchWindows, raise: switchRaise,
		closeWin: switchClose, quit: switchQuit, render: switchPNG, listPNG: switchListPNG, bigPNG: switchBigPNG,
		idlePNG: switchRecentPNG, badges: unreadBadges, frontPid: switchFrontPid}
}

func (w *switcher) open() bool {
	return time.Now().Before(w.openTill)
}

func (w *switcher) isOpen() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.open()
}

func (w *switcher) close() {
	w.mu.Lock()
	w.openTill = time.Time{}
	w.mu.Unlock()
	w.publish()
}

func (w *switcher) touch() {
	w.openTill = time.Now().Add(switchOpenFor)
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *switcher) tileDown() error {
	w.mu.Lock()
	if w.open() {
		w.tileHeld = true
		w.holdStart()
		w.mu.Unlock()
		return nil
	}
	defer w.mu.Unlock()
	return w.openLocked()
}

func (w *switcher) tileUp() error {
	w.mu.Lock()
	took := w.tileHeld
	w.tileHeld = false
	w.mu.Unlock()
	if took {
		w.click()
	}
	return nil
}

func (w *switcher) openLocked() error {
	o, err := w.orient()
	if err != nil {
		return err
	}
	apps, err := w.list()
	if err != nil {
		return err
	}
	if len(apps) == 0 {
		return fmt.Errorf("switcher: no apps")
	}
	left, right := w.strips[emojiLeft], w.strips[emojiRight]
	*left = emojiStrip{turn: upright(o, false), mirror: stripFlipped(o, false)}
	*right = emojiStrip{}
	w.turned = o != 0
	w.apps = apps
	for _, a := range apps {
		left.list = append(left.list, strconv.Itoa(a.pid))
	}
	w.app, w.titles, w.loaded, w.tile, w.front, w.want = 0, nil, false, "", -1, [2]int{-1, -1}
	w.post(switchJob{kind: jobList, pid: apps[0].pid})
	w.touch()
	return nil
}

func (w *switcher) post(j switchJob) {
	j.asked = time.Now()
	for {
		select {
		case w.jobs <- j:
			return
		default:
		}
		select {
		case <-w.jobs:
		default:
		}
	}
}

// work runs every AX call on one goroutine, in order.
func (w *switcher) work() {
	for j := range w.jobs {
		var err error
		switch j.kind {
		case jobWindow:
			err = w.raise(j.pid, j.window)
		case jobApp:
			err = w.raise(j.pid, -1)
		case jobClose:
			err = w.closeWin(j.pid, j.window)
		case jobQuit:
			err = w.quit(j.pid)
		}
		if j.kind != jobList {
			log.Printf("switcher: %s pid %d window %d in %s: %v", j.kind, j.pid, j.window, time.Since(j.asked).Round(time.Millisecond), err)
		}
		if j.kind == jobWindow || j.kind == jobQuit {
			continue
		}
		read := time.Now()
		titles, err := w.windows(j.pid)
		if took := time.Since(read); err != nil || took > 100*time.Millisecond {
			log.Printf("switcher: %d windows of %d in %s: %v", len(titles), j.pid, took.Round(time.Millisecond), err)
		}
		w.mu.Lock()
		if w.open() && w.app >= 0 && w.app < len(w.apps) && w.apps[w.app].pid == j.pid {
			keep := 0.0
			if j.kind == jobClose {
				keep = w.strips[emojiRight].target
			}
			right := w.strips[emojiRight]
			*right = emojiStrip{list: make([]string, len(titles))}
			right.offset = right.clamp(keep)
			right.target = right.offset
			w.titles, w.loaded = titles, true
			w.touch()
		}
		w.mu.Unlock()
	}
}

func (w *switcher) turn(side, direction int) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.open() {
		return false
	}
	if w.turned {
		direction = -direction
	}
	w.strips[side].knob(direction)
	w.touch()
	return true
}

func (w *switcher) press(side int) (bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.knobDown[side] = w.open()
	if w.knobDown[side] {
		w.holdStart()
	}
	return w.knobDown[side], nil
}

func (w *switcher) release(side int) bool {
	w.mu.Lock()
	took := w.knobDown[side]
	w.knobDown[side] = false
	w.mu.Unlock()
	if took {
		w.click()
	}
	return took
}

// click ends a press: a short one brings the ring pick to the front and closes the switcher, a hold has acted already.
func (w *switcher) click() {
	w.mu.Lock()
	w.holdGen++
	held := w.held
	if !held && w.open() {
		w.commit()
	}
	w.mu.Unlock()
	if !held {
		w.close()
	}
}

func (w *switcher) ring(side int) int {
	return int(math.Round(w.strips[side].target))
}

func (w *switcher) commit() {
	app := w.ring(emojiLeft)
	if app != w.app || !w.loaded || len(w.titles) == 0 {
		w.post(switchJob{kind: jobApp, pid: w.apps[app].pid})
		return
	}
	w.post(switchJob{kind: jobWindow, pid: w.apps[app].pid, window: w.ring(emojiRight)})
}

func (w *switcher) holdStart() {
	w.holdGen++
	w.held = false
	gen := w.holdGen
	time.AfterFunc(switchHoldClose, func() { w.holdStep(gen, false) })
}

// holdStep: a held tile or knob closes the ring window, held switchHoldQuit longer it quits the app.
func (w *switcher) holdStep(gen int, quit bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if gen != w.holdGen || !w.open() || w.app < 0 {
		return
	}
	w.held = true
	w.touch()
	pid := w.apps[w.app].pid
	if !quit {
		if w.loaded && len(w.titles) > 0 {
			w.post(switchJob{kind: jobClose, pid: pid, window: w.ring(emojiRight)})
		}
		time.AfterFunc(switchHoldQuit, func() { w.holdStep(gen, true) })
		return
	}
	w.post(switchJob{kind: jobQuit, pid: pid})
	w.apps = append(w.apps[:w.app:w.app], w.apps[w.app+1:]...)
	left := w.strips[emojiLeft]
	left.list = append(left.list[:w.app:w.app], left.list[w.app+1:]...)
	if len(w.apps) == 0 {
		w.openTill = time.Time{}
		return
	}
	left.offset = left.clamp(left.offset)
	left.target = left.clamp(left.target)
	*w.strips[emojiRight] = emojiStrip{}
	w.app, w.titles, w.loaded = -1, nil, false
}

func (w *switcher) start(side int, y float64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.open() {
		return
	}
	w.strips[side].begin(y)
	w.touch()
}

func (w *switcher) slide(side int, y float64) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.open() {
		return false
	}
	w.strips[side].slideTo(y)
	w.touch()
	return true
}

func (w *switcher) drag(side int, dy float64) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.open() {
		return false
	}
	w.strips[side].dragBy(dy)
	w.touch()
	return true
}

// end: a tap on the left strip brings the tapped app to the ring and to the front; a tap on the right one raises the ring window.
func (w *switcher) end(side int) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.open() {
		return nil
	}
	s := w.strips[side]
	if tap, rows := s.finish(); tap {
		if side == emojiLeft {
			s.knob(int(rows))
		}
		w.want[side] = w.ring(side)
	}
	w.touch()
	return nil
}

func (w *switcher) animate() {
	for range w.wake {
		ticker := time.NewTicker(time.Second / animationHz)
		for range ticker.C {
			if !w.frame() {
				break
			}
		}
		ticker.Stop()
	}
}

func (w *switcher) frame() bool {
	w.mu.Lock()
	open := w.open()
	for _, s := range w.strips {
		s.step()
	}
	if open {
		w.pick()
	}
	w.mu.Unlock()
	w.publish()
	return open
}

// pick: a settled left strip loads the windows of its app; only a tap raises anything.
func (w *switcher) pick() {
	if app, ok := w.strips[emojiLeft].settled(); ok {
		tapped := w.want[emojiLeft] == app
		w.want[emojiLeft] = -1
		switch {
		case app != w.app:
			*w.strips[emojiRight] = emojiStrip{}
			w.app, w.titles, w.loaded = app, nil, false
			kind := jobList
			if tapped {
				kind = jobApp
			}
			w.post(switchJob{kind: kind, pid: w.apps[app].pid})
			return
		case tapped:
			w.post(switchJob{kind: jobApp, pid: w.apps[app].pid})
		}
	}
	if window, ok := w.strips[emojiRight].settled(); ok && w.loaded && w.want[emojiRight] == window {
		w.want[emojiRight] = -1
		w.post(switchJob{kind: jobWindow, pid: w.apps[w.app].pid, window: window})
	}
}

func (w *switcher) rows() []string {
	rows := make([]string, len(w.titles))
	for i, t := range w.titles {
		rows[i] = strings.ToLower(cmp.Or(t, w.apps[w.app].name))
	}
	return rows
}

// idle redraws the closed tile: the apps of the newest windows; the list is read again only when the front app changes or the switcher closes.
func (w *switcher) idle() {
	w.mu.Lock()
	if w.open() {
		w.mu.Unlock()
		return
	}
	front := w.frontPid()
	relist := w.idleKey == "" || front != w.front
	w.mu.Unlock()
	var apps []switchApp
	if relist {
		all, err := w.list()
		if err != nil {
			log.Print(err)
			return
		}
		apps = all[:min(len(all), switchRecent)]
	} else {
		apps = w.recentApps()
	}
	dock := w.badges()
	badges := make([]bool, len(apps))
	key := ""
	for i, a := range apps {
		badges[i] = dock[a.name] != ""
		key += fmt.Sprintf("%d %s %t\n", a.pid, a.name, badges[i])
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.open() {
		return
	}
	w.front = front
	if key == w.idleKey {
		return
	}
	uri, err := w.idlePNG(apps, badges)
	if err != nil {
		log.Print(err)
		return
	}
	w.idleKey, w.idleTile, w.recent = key, uri, apps
	w.vars.set("switch_tile", uri)
}

func (w *switcher) recentApps() []switchApp {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.recent
}

func (w *switcher) publish() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.open() {
		w.overlay.show("switch", false)
		w.vars.set("switch_tile", w.idleTile)
		return
	}
	for side, s := range w.strips {
		key := fmt.Sprintf("%.2f %d %d", s.offset, len(s.list), w.app)
		if s.shown == key {
			continue
		}
		s.shown = key
		var uri string
		var err error
		if side == emojiLeft {
			uri, err = w.render(emojiColumn(s.list, s.offset, s.turn, s.mirror), emojiRingY(s.mirror))
		} else {
			uri, err = w.listPNG(w.rows(), s.offset)
		}
		if err != nil {
			log.Print(err)
			continue
		}
		w.vars.set([2]string{"emoji_l", "emoji_r"}[side], uri)
	}
	if w.app < 0 {
		return
	}
	if pid := strconv.Itoa(w.apps[w.app].pid); pid != w.tile {
		w.tile = pid
		uri, err := w.bigPNG(w.apps[w.app].pid)
		if err != nil {
			log.Print(err)
		}
		w.vars.set("switch_tile", uri)
	}
	w.overlay.show("switch", true)
}
