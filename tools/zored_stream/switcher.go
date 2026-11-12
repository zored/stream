package main

import (
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
)

type switchApp struct {
	pid  int
	name string
}

type switchJob struct {
	pid      int
	window   int
	activate bool
	asked    time.Time
}

// switcher: the left strip picks an app, the right one a window of it; a pick comes to the front as soon as the strip settles.
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
	window   int
	tile     string
	card     string
	front    int
	idleKey  string
	idleTile string
	recent   []switchApp
	knobDown [2]bool
	wake     chan struct{}
	jobs     chan switchJob
	orient   func() (int, error)
	list     func() ([]switchApp, error)
	windows  func(pid int) ([]string, error)
	raise    func(pid, window int) error
	render   func(side int, glyphs []emojiGlyph, ringY float64) (string, error)
	cardPNG  func(text string) (string, error)
	bigPNG   func(pid int) (string, error)
	idlePNG  func(apps []switchApp, badges []bool) (string, error)
	badges   func() map[string]string
	frontPid func() int
}

var switchIdle = tileIcon("window-stack", "#fff")

func newSwitcher(v *vars, o *stripOverlay) *switcher {
	return &switcher{vars: v, overlay: o, wake: make(chan struct{}, 1), jobs: make(chan switchJob, 1), strips: [2]*emojiStrip{{}, {}},
		idleTile: switchIdle, orient: readOrientation, list: switchApps, windows: switchWindows, raise: switchRaise, render: switchPNG,
		cardPNG: switchCardPNG, bigPNG: switchBigPNG, idlePNG: switchRecentPNG, badges: unreadBadges, frontPid: switchFrontPid}
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

func (w *switcher) pressTile() error {
	w.mu.Lock()
	if w.open() {
		w.mu.Unlock()
		w.close()
		return nil
	}
	defer w.mu.Unlock()
	o, err := w.orient()
	if err != nil {
		return err
	}
	apps, err := w.list()
	if err != nil {
		return err
	}
	if len(apps) == 0 {
		return fmt.Errorf("switcher: no apps with windows")
	}
	left, right := w.strips[emojiLeft], w.strips[emojiRight]
	*left = emojiStrip{turn: upright(o, false), mirror: stripFlipped(o, false)}
	*right = emojiStrip{turn: upright(o, true)}
	w.turned = o != 0
	w.apps = apps
	for _, a := range apps {
		left.list = append(left.list, strconv.Itoa(a.pid))
	}
	w.app, w.window, w.titles, w.loaded, w.tile, w.card, w.front = 0, 0, nil, false, "", "", -1
	w.post(switchJob{pid: apps[0].pid, window: -1})
	w.touch()
	return nil
}

func (w *switcher) post(j switchJob) {
	j.asked = time.Now()
	select {
	case <-w.jobs:
	default:
	}
	w.jobs <- j
}

// work runs every AX call on one goroutine: the window list of the picked app, then the raises.
func (w *switcher) work() {
	for j := range w.jobs {
		if j.window >= 0 {
			w.raiseLogged(j)
			continue
		}
		if j.activate {
			w.raiseLogged(j)
		}
		titles, err := w.windows(j.pid)
		if err != nil {
			log.Printf("switcher: windows of %d: %v", j.pid, err)
		}
		w.mu.Lock()
		if w.app < len(w.apps) && w.apps[w.app].pid == j.pid {
			right := w.strips[emojiRight]
			*right = emojiStrip{turn: right.turn, list: make([]string, len(titles))}
			w.titles, w.loaded, w.window = titles, true, 0
			w.touch()
		}
		w.mu.Unlock()
	}
}

func (w *switcher) raiseLogged(j switchJob) {
	err := w.raise(j.pid, j.window)
	log.Printf("switcher: raise pid %d window %d in %s: %v", j.pid, j.window, time.Since(j.asked).Round(time.Millisecond), err)
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
	w.knobDown[side] = w.open()
	took := w.knobDown[side]
	w.mu.Unlock()
	if took {
		w.close()
	}
	return took, nil
}

func (w *switcher) release(side int) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	took := w.knobDown[side]
	w.knobDown[side] = false
	return took
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

func (w *switcher) end(side int) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.open() {
		return nil
	}
	s := w.strips[side]
	if tap, rows := s.finish(); tap {
		s.knob(int(rows))
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

func (w *switcher) pick() {
	if app, ok := w.strips[emojiLeft].settled(); ok && app != w.app {
		right := w.strips[emojiRight]
		*right = emojiStrip{turn: right.turn}
		w.app, w.window, w.titles, w.loaded = app, 0, nil, false
		w.post(switchJob{pid: w.apps[app].pid, window: -1, activate: true})
		return
	}
	if window, ok := w.strips[emojiRight].settled(); ok && w.loaded && window != w.window {
		w.window = window
		w.post(switchJob{pid: w.apps[w.app].pid, window: window})
	}
}

func (w *switcher) cardText() string {
	s := w.strips[emojiRight]
	if len(s.list) == 0 {
		return ""
	}
	i := int(math.Round(s.clamp(s.offset)))
	if i < len(w.titles) && w.titles[i] != "" {
		return strings.ToLower(w.titles[i])
	}
	return strings.ToLower(w.apps[w.app].name)
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
	if text := w.cardText(); text != w.card {
		w.card = text
		uri, err := w.cardPNG(text)
		if err != nil {
			log.Print(err)
		}
		w.vars.set("emoji_r", uri)
	}
	for side, s := range w.strips {
		if side == emojiRight {
			continue
		}
		key := fmt.Sprintf("%.2f %d", s.offset, len(s.list))
		if s.shown == key {
			continue
		}
		s.shown = key
		uri, err := w.render(side, emojiColumn(s.list, s.offset, s.turn, s.mirror), emojiRingY(s.mirror))
		if err != nil {
			log.Print(err)
			continue
		}
		w.vars.set([2]string{"emoji_l", "emoji_r"}[side], uri)
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
