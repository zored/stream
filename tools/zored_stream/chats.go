package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"html"
	"log"
	"net/http"
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
	c.list = chatsMerge(c.byApp)
	c.pick = min(c.pick, max(len(c.list)-1, 0))
	c.publishLocked()
}

func chatsMerge(byApp map[string][]chatEntry) []chatEntry {
	var out []chatEntry
	for _, l := range byApp {
		for _, e := range l {
			if e.unread > 0 {
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
		svg = chatsPickSVG(c.list, c.pick, now)
		c.strip(chatsStripImage(c.list, c.pick))
	} else {
		if c.covered {
			c.covered = false
			c.strip("")
		}
		svg = chatsTileSVG(c.list[:min(len(c.list), chatsOnTile)], now)
	}
	if svg != c.shown {
		c.shown = svg
		c.vars.set("unread_svg", svg)
	}
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

func chatsAvatar(e chatEntry, cx, cy, r float64, id string) string {
	if len(e.avatar) == 0 {
		return fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="#333"/>`+
			`<text x="%.1f" y="%.1f" text-anchor="middle" font-family="JetBrains Mono" font-size="%.1f" fill="#fff">%s</text>`,
			cx, cy, r, cx, cy+r*0.35, r, html.EscapeString(chatsInitial(e.title)))
	}
	mime := http.DetectContentType(e.avatar)
	return fmt.Sprintf(`<clipPath id="%s"><circle cx="%.1f" cy="%.1f" r="%.1f"/></clipPath>`+
		`<image href="data:%s;base64,%s" x="%.1f" y="%.1f" width="%.1f" height="%.1f" clip-path="url(#%s)" preserveAspectRatio="xMidYMid slice"/>`,
		id, cx, cy, r, mime, base64.StdEncoding.EncodeToString(e.avatar), cx-r, cy-r, 2*r, 2*r, id)
}

func chatsLabel(x, y, size float64, rgb, s string) string {
	return fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="middle" font-family="JetBrains Mono" font-size="%.1f" fill="%s">%s</text>`, x, y, size, rgb, s)
}

func chatsTileSVG(list []chatEntry, now time.Time) string {
	if len(list) == 0 {
		return unreadSVG(nil, now)
	}
	var b strings.Builder
	step := 88.0 / float64(len(list))
	for i, e := range list {
		cx := 6 + step*(float64(i)+0.5)
		b.WriteString(chatsAvatar(e, cx, 32, 13, fmt.Sprintf("a%d", i)))
		b.WriteString(chatsLabel(cx, 64, 15, "#fff", capped(e.unread, 99)))
		b.WriteString(chatsLabel(cx, 84, 13, "#808080", ago(e.at, now)))
	}
	return dataURI(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">` + b.String() + `</svg>`)
}

func chatsPickSVG(list []chatEntry, pick int, now time.Time) string {
	if pick >= len(list) {
		return unreadSVG(nil, now)
	}
	e := list[pick]
	return dataURI(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">` +
		chatsAvatar(e, 50, 36, 28, "p") +
		chatsLabel(30, 88, 15, "#fff", capped(e.unread, 99)) +
		chatsLabel(70, 88, 15, "#808080", ago(e.at, now)) +
		`</svg>`)
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
