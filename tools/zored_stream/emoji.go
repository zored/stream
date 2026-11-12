package main

import (
	"fmt"
	"log"
	"math"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	emojiOpenFor  = 4 * time.Second
	emojiTop      = 26
	emojiRing     = 22
	emojiPitch    = 43
	emojiSize     = 30
	emojiTileSize = 64
	emojiTapPx    = 6
	emojiFriction = 0.94
	emojiFling    = 2.0
	emojiSnap     = 0.25
)

const (
	emojiLeft = iota
	emojiRight
)

type emojiStrip struct {
	list           []string
	offset, target float64
	vel            float64
	touching       bool
	lastY          float64
	hasY           bool
	moved          float64
	lastMove       time.Time
	knobDown       bool
	accel          accel
	shown          string
	turn           float64
	mirror         bool
}

type emoji struct {
	mu       sync.Mutex
	vars     *vars
	overlay  *stripOverlay
	openTill time.Time
	strips   [2]*emojiStrip
	active   int
	turned   bool
	starts   []int
	idle     string
	tileText string
	wake     chan struct{}
	recents  func() ([]string, error)
	orient   func() (int, error)
	send     func(string) error
	render   func(glyphs []emojiGlyph, ringY float64) (string, error)
}

func newEmoji(v *vars, o *stripOverlay) *emoji {
	return &emoji{vars: v, overlay: o, idle: emojiIdle, wake: make(chan struct{}, 1), recents: emojiRecents, orient: readOrientation, send: emojiType, render: emojiPNG,
		strips: [2]*emojiStrip{{}, {}}}
}

func (e *emoji) open() bool {
	return time.Now().Before(e.openTill)
}

func (e *emoji) isOpen() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.open()
}

func (e *emoji) close() {
	e.mu.Lock()
	e.openTill = time.Time{}
	e.mu.Unlock()
	e.publish()
}

func (e *emoji) touch() {
	e.openTill = time.Now().Add(emojiOpenFor)
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

func (e *emoji) tile() error {
	e.mu.Lock()
	if e.open() {
		s := e.top(emojiLeft)
		e.touch()
		e.mu.Unlock()
		return e.sendLogged(s)
	}
	defer e.mu.Unlock()
	o, err := e.orient()
	if err != nil {
		return err
	}
	e.strips[emojiLeft].turn, e.strips[emojiRight].turn = upright(o, false), upright(o, true)
	e.strips[emojiLeft].mirror = stripFlipped(o, false)
	e.turned = o != 0
	recents, err := e.recents()
	if err != nil {
		log.Print(err)
	}
	e.idle = emojiIdleImage(recents)
	left := e.strips[emojiLeft]
	left.list = slices.Concat(append([][]string{recents}, emojiGroups...)...)
	e.starts = []int{0}
	next := len(recents)
	for _, g := range emojiGroups {
		e.starts = append(e.starts, next)
		next += len(g)
	}
	e.strips[emojiRight].list = emojiCategories
	for _, s := range e.strips {
		s.offset, s.target, s.vel, s.shown = 0, 0, 0, ""
	}
	e.active, e.tileText = emojiLeft, ""
	e.touch()
	return nil
}

func (e *emoji) top(side int) string {
	s := e.strips[side]
	if len(s.list) == 0 {
		return ""
	}
	return s.list[min(max(int(math.Round(s.offset)), 0), len(s.list)-1)]
}

func (e *emoji) sendLogged(s string) error {
	if s == "" {
		return nil
	}
	log.Printf("emoji: type %s", s)
	return e.send(s)
}

func (e *emoji) turn(side, direction int) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.open() {
		return false
	}
	s := e.strips[side]
	if e.turned {
		direction = -direction
	}
	s.knob(s.accel.step(direction))
	e.active = side
	e.touch()
	return true
}

func (e *emoji) press(side int) (bool, error) {
	e.mu.Lock()
	s := e.strips[side]
	s.knobDown = e.open()
	if !s.knobDown {
		e.mu.Unlock()
		return false, nil
	}
	e.active = side
	text := e.top(emojiLeft)
	e.touch()
	e.mu.Unlock()
	return true, e.sendLogged(text)
}

func (e *emoji) release(side int) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	took := e.strips[side].knobDown
	e.strips[side].knobDown = false
	return took
}

func (e *emoji) start(side int, y float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.open() {
		return
	}
	e.strips[side].begin(y)
	e.active = side
	e.touch()
}

func (e *emoji) slide(side int, y float64) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.open() {
		return false
	}
	e.strips[side].slideTo(y)
	e.active = side
	e.touch()
	return true
}

func (e *emoji) drag(side int, dy float64) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.open() {
		return false
	}
	e.dragLocked(side, dy)
	return true
}

func (e *emoji) dragLocked(side int, dy float64) {
	e.strips[side].dragBy(dy)
	e.active = side
	e.touch()
}

func (e *emoji) end(side int) error {
	e.mu.Lock()
	if !e.open() {
		e.mu.Unlock()
		return nil
	}
	s := e.strips[side]
	tap, rows := s.finish()
	e.touch()
	if !tap {
		e.mu.Unlock()
		return nil
	}
	if rows == 0 && side == emojiLeft {
		text := e.top(emojiLeft)
		e.mu.Unlock()
		return e.sendLogged(text)
	}
	s.knob(int(rows))
	e.active = side
	e.mu.Unlock()
	return nil
}

func (e *emoji) animate() {
	for range e.wake {
		ticker := time.NewTicker(time.Second / animationHz)
		for range ticker.C {
			if !e.frame() {
				break
			}
		}
		ticker.Stop()
	}
}

func (e *emoji) frame() bool {
	e.mu.Lock()
	open := e.open()
	for _, s := range e.strips {
		s.step()
	}
	e.sync()
	e.mu.Unlock()
	e.publish()
	return open
}

func (e *emoji) sync() {
	left, right := e.strips[emojiLeft], e.strips[emojiRight]
	if len(e.starts) == 0 {
		return
	}
	if e.active == emojiRight {
		start := float64(e.starts[int(math.Round(right.target))])
		if left.target != start {
			left.target, left.vel = start, 0
		}
		return
	}
	at := int(math.Round(left.offset))
	cat := 0
	for i, start := range e.starts {
		if start <= at {
			cat = i
		}
	}
	if !right.touching && right.vel == 0 {
		right.target = float64(cat)
	}
}

func (e *emoji) publish() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.open() {
		e.overlay.show("emoji", false)
		e.vars.set("emoji_tile", e.idle)
		return
	}
	for side, s := range e.strips {
		key := fmt.Sprintf("%.2f %d", s.offset, len(s.list))
		if s.shown == key {
			continue
		}
		s.shown = key
		uri, err := e.render(emojiColumn(s.list, s.offset, s.turn, s.mirror), emojiRingY(s.mirror))
		if err != nil {
			log.Print(err)
			continue
		}
		if side == emojiLeft {
			e.vars.set("emoji_l", uri)
		} else {
			e.vars.set("emoji_r", uri)
		}
	}
	if text := e.top(emojiLeft); text != e.tileText {
		e.tileText = text
		e.vars.set("emoji_tile", emojiTileImage(text, emojiTileSize))
	}
	e.overlay.show("emoji", true)
}

func emojiRingY(mirror bool) float64 {
	if mirror {
		return stripDrawH - emojiTop
	}
	return emojiTop
}

func emojiColumn(list []string, offset, turn float64, mirror bool) []emojiGlyph {
	var glyphs []emojiGlyph
	first := int(math.Floor(offset))
	for i := first; i < len(list); i++ {
		if i < 0 {
			continue
		}
		y := emojiTop + (float64(i)-offset)*emojiPitch
		if y > stripDrawH+emojiPitch {
			break
		}
		if mirror {
			y = stripDrawH - y
		}
		glyphs = append(glyphs, emojiGlyph{text: list[i], y: y, size: emojiSize, alpha: 1, turn: turn})
	}
	return glyphs
}

var emojiCategories = []string{"🕘", "😀", "👋", "🐻", "🍔", "⚽", "🚗", "💡", "🔣", "🏁"}

var emojiIdle = tileIcon("emoji-smile", "#fff")

func emojiIdleImage(recents []string) string {
	return captionTile("emoji-smile", "#fff", strings.Join(recents[:min(3, len(recents))], ""), "Apple Color Emoji")
}

func emojiTileImage(text string, size float64) string {
	uri, err := emojiTilePNG(text, size)
	if err != nil {
		log.Print(err)
	}
	return uri
}
