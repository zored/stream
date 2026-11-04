package main

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	chatsBrowseFor = 5 * time.Second
	chatsOnTile    = 4
	chatsRetry     = 30 * time.Second
	chatsHold      = 400 * time.Millisecond
)

type chatEntry struct {
	app    string
	id     string
	title  string
	unread int
	at     time.Time
	text   string
	avatar []byte
	open   string
	muted  bool
}

type chatSource func(ctx context.Context, push func([]chatEntry)) error

type chats struct {
	mu         sync.Mutex
	vars       *vars
	release    func()
	strip      func(uri string)
	byApp      map[string][]chatEntry
	list       []chatEntry
	browseTill time.Time
	covered    bool
	pick       int
	knobDown   bool
	opener     func(url string) error
	muted      bool
	hold       *time.Timer
	held       bool
	slideGen   int
	sliding    bool
}

func newChats(v *vars, release func(), strip func(string)) *chats {
	return &chats{vars: v, release: release, strip: strip, byApp: map[string][]chatEntry{}, opener: openURL}
}

func (c *chats) run(ctx context.Context, app string, src chatSource) {
	for ctx.Err() == nil {
		err := src(ctx, func(list []chatEntry) { c.set(app, list) })
		log.Printf("chats %s: %v", app, err)
		c.set(app, nil)
		select {
		case <-ctx.Done():
		case <-time.After(chatsRetry):
		}
	}
}

func (c *chats) set(app string, list []chatEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byApp[app] = list
	c.list = chatsMerge(c.byApp, c.muted)
	c.pick = min(c.pick, max(len(c.list)-1, 0))
	c.publishLocked()
}

func chatsMerge(byApp map[string][]chatEntry, muted bool) []chatEntry {
	var out []chatEntry
	for _, l := range byApp {
		for _, e := range l {
			if e.unread > 0 && e.muted == muted {
				out = append(out, e)
			}
		}
	}
	slices.SortStableFunc(out, func(a, b chatEntry) int { return b.at.Compare(a.at) })
	return out
}

func (c *chats) browsing() bool {
	return time.Now().Before(c.browseTill)
}

func (c *chats) publish() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.publishLocked()
}

func (c *chats) publishLocked() {
	now := time.Now()
	var svg string
	if c.sliding {
		return
	}
	if c.browsing() {
		c.covered = true
		svg = chatsPickImage(c.list, c.pick, now, c.muted)
		c.strip(chatsStripImage(c.list, c.pick))
	} else {
		if c.covered {
			c.covered = false
			c.strip("")
		}
		svg = chatsTileImage(c.list[:min(len(c.list), chatsOnTile)], len(c.list) > chatsOnTile, now, c.muted)
	}
	c.vars.set("unread_svg", svg)
}

func (c *chats) down() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.held = false
	c.hold = time.AfterFunc(chatsHold, c.toggleMuted)
	return nil
}

func (c *chats) up() error {
	c.mu.Lock()
	if c.hold != nil {
		c.hold.Stop()
		c.hold = nil
	}
	held := c.held
	c.mu.Unlock()
	if held {
		return nil
	}
	return c.press()
}

func (c *chats) toggleMuted() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.held = true
	c.muted = !c.muted
	c.list = chatsMerge(c.byApp, c.muted)
	c.pick, c.browseTill = 0, time.Time{}
	log.Printf("chats: muted=%v", c.muted)
	c.publishLocked()
}

func (c *chats) press() error {
	c.mu.Lock()
	if c.browsing() {
		i := c.pick
		c.mu.Unlock()
		return c.act(i)
	}
	if len(c.list) == 0 {
		c.mu.Unlock()
		return nil
	}
	c.pick, c.browseTill = 0, time.Now().Add(chatsBrowseFor)
	go c.release()
	c.publishLocked()
	c.mu.Unlock()
	return nil
}

func (c *chats) turn(direction int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.browsing() {
		return false
	}
	from := c.pick
	if n := len(c.list); n > 0 {
		c.pick = min(max(c.pick+direction, 0), n-1)
	}
	c.browseTill = time.Now().Add(chatsBrowseFor)
	if c.pick == from {
		c.publishLocked()
		return true
	}
	c.slideGen++
	c.sliding = true
	c.strip(chatsStripImage(c.list, c.pick))
	go c.slide(c.slideGen, c.list[from], c.list[c.pick], direction)
	return true
}

func (c *chats) slide(gen int, from, to chatEntry, direction int) {
	for i := 1; i <= chatsFrames; i++ {
		time.Sleep(chatsFrameAt)
		c.mu.Lock()
		if gen != c.slideGen {
			c.mu.Unlock()
			return
		}
		if i < chatsFrames {
			t := float64(i) / chatsFrames
			img := chatsSlideImage(from, to, 1-(1-t)*(1-t), direction, time.Now(), c.muted)
			c.vars.set("unread_svg", img)
		} else {
			c.sliding = false
			c.publishLocked()
		}
		c.mu.Unlock()
	}
}

func (c *chats) knob(down bool) (bool, error) {
	c.mu.Lock()
	if down {
		c.knobDown = c.browsing()
		c.mu.Unlock()
		return c.knobDown, nil
	}
	if !c.knobDown {
		c.mu.Unlock()
		return false, nil
	}
	c.knobDown = false
	i := c.pick
	c.mu.Unlock()
	return true, c.act(i)
}

func (c *chats) act(i int) error {
	c.mu.Lock()
	if i < 0 || i >= len(c.list) {
		c.mu.Unlock()
		return nil
	}
	e := c.list[i]
	c.browseTill = time.Time{}
	c.publishLocked()
	c.mu.Unlock()
	log.Printf("chats: open %s %s", e.app, e.id)
	return c.opener(e.open)
}

func openURL(url string) error {
	if url == "" {
		return fmt.Errorf("chats: no url to open")
	}
	return exec.Command("open", url).Run()
}

func chatsInitial(title string) string {
	for _, r := range title {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return strings.ToLower(string(r))
		}
	}
	return "·"
}

const (
	chatsRed     = 0xE53935
	chatsDark    = 0x333333
	chatsFrames  = 6
	chatsFrameAt = 25 * time.Millisecond
)

func chatsLabel(text string, cx, cy, size float64, rgb uint32) calText {
	lh := size * 1.25
	return calText{text: text, x: cx - 30, y: cy - lh/2, w: 60, h: lh, size: size, rgb: rgb, center: true}
}

func chatsBadges(e chatEntry, x, y, w, r, size float64, muted bool, now time.Time) ([]chatsDisc, []calText) {
	count := uint32(chatsRed)
	if muted {
		count = grayRGB
	}
	cx, top, bottom := x+w-r, y+r, y+w-r
	return []chatsDisc{{cx: cx, cy: top, r: r, rgb: count}, {cx: cx, cy: bottom, r: r, rgb: chatsDark}},
		[]calText{chatsLabel(capped(e.unread, 99), cx, top, size, whiteRGB), chatsLabel(ago(e.at, now), cx, bottom, size*0.85, whiteRGB)}
}

func chatsFace(e chatEntry, cx, cy, r float64, square bool) (chatsDisc, []calText) {
	d := chatsDisc{avatar: e.avatar, cx: cx, cy: cy, r: r, square: square}
	if len(e.avatar) > 0 {
		return d, nil
	}
	return d, []calText{chatsLabel(chatsInitial(e.title), cx, cy, r, whiteRGB)}
}

func chatsEmpty(now time.Time, muted bool) string {
	if muted {
		return dataURI(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">` + iconAt("eye-slash", gray, 50, 50, 44) + `</svg>`)
	}
	return unreadSVG(nil, now)
}

func chatsTileImage(list []chatEntry, more bool, now time.Time, muted bool) string {
	if len(list) == 0 {
		return chatsEmpty(now, muted)
	}
	if len(list) == 1 {
		d, t := chatsCard(list[0], 0, now, muted)
		return chatsPNG(88, 88, d, t)
	}
	if len(list) == 2 {
		var discs []chatsDisc
		var texts []calText
		for i, e := range list {
			o := 40 * float64(i)
			d, t := chatsFace(e, o+22, o+26, 21, false)
			bd, bt := chatsBadges(e, o, o, 48, 9, 9, muted, now)
			discs, texts = append(append(discs, d), bd...), append(append(texts, t...), bt...)
		}
		return chatsPNG(88, 88, discs, texts)
	}
	grid := 88.0
	var texts []calText
	if more {
		grid = 76
		texts = append(texts, chatsLabel("+", 82, 44, 16, grayRGB))
	}
	cell := grid / 2
	var discs []chatsDisc
	for i, e := range list {
		x, y := cell*float64(i%2), cell*float64(i/2)+(88-grid)/2
		d, t := chatsFace(e, x+cell*0.42, y+cell*0.58, cell*0.36, false)
		bd, bt := chatsBadges(e, x, y, cell, cell*0.2, cell*0.2, muted, now)
		discs, texts = append(append(discs, d), bd...), append(append(texts, t...), bt...)
	}
	return chatsPNG(88, 88, discs, texts)
}

func chatsCard(e chatEntry, dx float64, now time.Time, muted bool) ([]chatsDisc, []calText) {
	d, t := chatsFace(e, 44+dx, 44, 44, true)
	bd, bt := chatsBadges(e, dx, 0, 88, 13, 13, muted, now)
	return append([]chatsDisc{d}, bd...), append(t, bt...)
}

func chatsPickImage(list []chatEntry, pick int, now time.Time, muted bool) string {
	if pick >= len(list) {
		return chatsEmpty(now, muted)
	}
	d, t := chatsCard(list[pick], 0, now, muted)
	return chatsPNG(88, 88, d, t)
}

func chatsSlideImage(from, to chatEntry, step float64, direction int, now time.Time, muted bool) string {
	shift := 88 * float64(direction)
	d1, t1 := chatsCard(from, -shift*step, now, muted)
	d2, t2 := chatsCard(to, shift*(1-step), now, muted)
	return chatsPNG(88, 88, append(d1, d2...), append(t1, t2...))
}

func chatsLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func chatsStripImage(list []chatEntry, pick int) string {
	const w, h = stripDrawH, stripDrawW
	if pick >= len(list) {
		return calImage(w, h, true, calText{text: "·", y: 12, w: w, h: 24, size: 16, rgb: grayRGB, center: true})
	}
	e := list[pick]
	title := strings.ToLower(chatsLine(e.title))
	if !utf8.ValidString(title) || title == "" {
		title = "·"
	}
	return calImage(w, h, true, calText{text: title, tail: " " + chatsLine(e.text), x: 4, y: 1, w: w - 8, h: 46, size: 12, rgb: grayRGB, tailRGB: whiteRGB})
}
