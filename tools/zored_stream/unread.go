package main

import (
	"fmt"
	"html"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	unreadPollEvery = 2 * time.Second
	unreadAttribute = 5 * time.Second
	unreadMaxChats  = 9
)

type unreadApp struct {
	dock, bundle, letter string
	badge                int
	roseAt               time.Time
	chats                []string
}

type unreadItem struct {
	app  *unreadApp
	text string
}

type unread struct {
	mu      sync.Mutex
	vars    *vars
	apps    []*unreadApp
	seen    map[string]bool
	cursor  int
	pressed time.Time
	shown   string
}

func newUnread(v *vars) *unread {
	return &unread{vars: v, seen: map[string]bool{}, apps: []*unreadApp{
		{dock: "Telegram", bundle: "ru.keepcoder.Telegram", letter: "t"},
		{dock: "VK WorkSpace Beta", bundle: "ru.mail.messenger-biz-avocado-desktop.beta", letter: "w"},
	}}
}

func badgeCount(label string) int {
	if label == "" {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSuffix(label, "+"))
	if err != nil {
		return 1
	}
	return n
}

func (u *unread) poll() {
	badges, banners := unreadScan()
	u.update(time.Now(), badges, banners)
}

func (u *unread) update(now time.Time, badges map[string]string, banners [][2]string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	for _, a := range u.apps {
		n := badgeCount(badges[a.dock])
		if n > a.badge {
			a.roseAt = now
		}
		if n == 0 {
			a.chats = nil
		}
		a.badge = n
	}
	seen := map[string]bool{}
	for _, b := range banners {
		seen[b[0]] = true
		if u.seen[b[0]] {
			continue
		}
		if a := u.owner(now); a != nil {
			a.add(b[1])
		}
	}
	u.seen = seen
	u.publishLocked()
}

func (u *unread) owner(now time.Time) *unreadApp {
	var best *unreadApp
	for _, a := range u.apps {
		if a.badge > 0 && now.Sub(a.roseAt) <= unreadAttribute && (best == nil || a.roseAt.After(best.roseAt)) {
			best = a
		}
	}
	return best
}

func (a *unreadApp) add(chat string) {
	out := []string{chat}
	for _, c := range a.chats {
		if c != chat && len(out) < unreadMaxChats {
			out = append(out, c)
		}
	}
	a.chats = out
}

func (u *unread) items() []unreadItem {
	var out []unreadItem
	for _, a := range u.apps {
		if a.badge == 0 {
			continue
		}
		if len(a.chats) == 0 {
			out = append(out, unreadItem{a, a.letter})
		}
		for _, c := range a.chats {
			out = append(out, unreadItem{a, c})
		}
	}
	return out
}

func (u *unread) current() (unreadItem, int, int, bool) {
	items := u.items()
	if len(items) == 0 {
		return unreadItem{}, 0, 0, false
	}
	i := u.cursor % len(items)
	return items[i], i, len(items), true
}

func (u *unread) publishLocked() {
	svg := ""
	if it, i, n, ok := u.current(); ok {
		svg = unreadSVG(initial(it.text), it.app.badge, i, n)
	}
	if svg != u.shown {
		u.shown = svg
		u.vars.set("unread_svg", svg)
	}
}

func initial(s string) string {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return string(unicode.ToLower(r))
		}
	}
	r, _ := utf8.DecodeRuneInString(s)
	return string(r)
}

func unreadSVG(letter string, count, index, total int) string {
	var dots strings.Builder
	if total > 1 {
		x0 := 50 - float64(total-1)*5
		for i := range total {
			fill := "#808080"
			if i == index {
				fill = "#fff"
			}
			fmt.Fprintf(&dots, `<circle cx="%.1f" cy="88" r="2.5" fill="%s"/>`, x0+float64(i)*10, fill)
		}
	}
	return dataURI(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">`+
		`<text x="50" y="44" dy="0.35em" text-anchor="middle" font-family="JetBrains Mono" font-size="52" fill="#fff">%s</text>`+
		`<text x="94" y="14" text-anchor="end" font-family="JetBrains Mono" font-size="16" fill="#808080">%d</text>%s</svg>`,
		html.EscapeString(letter), count, dots.String()))
}

func (u *unread) active() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.items()) > 0
}

func (u *unread) down() error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.pressed = time.Now()
	return nil
}

func (u *unread) up() error {
	u.mu.Lock()
	if u.pressed.IsZero() {
		u.mu.Unlock()
		return nil
	}
	held := time.Since(u.pressed) >= holdMin
	u.pressed = time.Time{}
	it, _, _, ok := u.current()
	if !held {
		u.cursor++
		u.publishLocked()
	}
	u.mu.Unlock()
	if held && ok {
		return unreadOpen(it.app.bundle)
	}
	return nil
}
