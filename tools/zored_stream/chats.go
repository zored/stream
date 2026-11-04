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
	chatsOnTile    = 3
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
	shown      string
	opener     func(url string) error
	muted      bool
	hold       *time.Timer
	held       bool
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
	if c.browsing() {
		c.covered = true
		svg = chatsPickImage(c.list, c.pick, now, c.muted)
		c.strip(chatsStripImage(c.list, c.pick))
	} else {
		if c.covered {
			c.covered = false
			c.strip("")
		}
		svg = chatsTileImage(c.list[:min(len(c.list), chatsOnTile)], now, c.muted)
	}
	if svg != c.shown {
		c.shown = svg
		c.vars.set("unread_svg", svg)
	}
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
	if n := len(c.list); n > 0 {
		c.pick = min(max(c.pick+direction, 0), n-1)
	}
	c.browseTill = time.Now().Add(chatsBrowseFor)
	c.publishLocked()
	return true
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

func chatsDraw(e chatEntry, cx, cy, r float64) (chatsDisc, []calText) {
	d := chatsDisc{avatar: e.avatar, cx: cx, cy: cy, r: r}
	if len(e.avatar) > 0 {
		return d, nil
	}
	return d, []calText{{text: chatsInitial(e.title), x: cx - r, y: cy - r*0.7, w: 2 * r, h: 1.4 * r, size: r, rgb: whiteRGB, center: true}}
}

func chatsCountRGB(muted bool) uint32 {
	if muted {
		return grayRGB
	}
	return whiteRGB
}

func chatsEmpty(now time.Time, muted bool) string {
	if muted {
		return chatsPNG(88, 88, nil, []calText{{text: "·", y: 33, w: 88, h: 22, size: 16, rgb: grayRGB, center: true}})
	}
	return unreadSVG(nil, now)
}

func chatsTileImage(list []chatEntry, now time.Time, muted bool) string {
	if len(list) == 0 {
		return chatsEmpty(now, muted)
	}
	var discs []chatsDisc
	var texts []calText
	step := 88.0 / float64(len(list))
	for i, e := range list {
		x := step * float64(i)
		d, t := chatsDraw(e, x+step/2, 24, 13)
		discs, texts = append(discs, d), append(texts, t...)
		texts = append(texts,
			calText{text: capped(e.unread, 99), x: x, y: 42, w: step, h: 20, size: 15, rgb: chatsCountRGB(muted), center: true},
			calText{text: ago(e.at, now), x: x, y: 64, w: step, h: 18, size: 13, rgb: grayRGB, center: true})
	}
	return chatsPNG(88, 88, discs, texts)
}

func chatsPickImage(list []chatEntry, pick int, now time.Time, muted bool) string {
	if pick >= len(list) {
		return chatsEmpty(now, muted)
	}
	e := list[pick]
	d, texts := chatsDraw(e, 44, 30, 28)
	texts = append(texts,
		calText{text: capped(e.unread, 99), y: 64, w: 44, h: 22, size: 15, rgb: chatsCountRGB(muted), center: true},
		calText{text: ago(e.at, now), x: 44, y: 64, w: 44, h: 22, size: 15, rgb: grayRGB, center: true})
	return chatsPNG(88, 88, []chatsDisc{d}, texts)
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
	return calImage(w, h, true, calText{text: title, tail: " " + chatsLine(e.text), x: 4, y: 3, w: w - 8, h: 44, size: 15, rgb: grayRGB, tailRGB: whiteRGB})
}
