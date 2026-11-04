package main

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

const unreadPollEvery = 2 * time.Second

type unreadApp struct {
	dock, bundle, fill string
	badge              int
	lastAt             time.Time
}

type unread struct {
	mu     sync.Mutex
	vars   *vars
	apps   []*unreadApp
	cursor int
	shown  string
	primed bool
}

func newUnread(v *vars) *unread {
	return &unread{vars: v, apps: []*unreadApp{
		{dock: "Telegram", bundle: "ru.keepcoder.Telegram", fill: "#2AABEE"},
		{dock: "VK WorkSpace Beta", bundle: "ru.mail.messenger-biz-avocado-desktop.beta", fill: "#1A3A8F"},
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
	u.update(time.Now(), unreadBadges())
}

func (u *unread) update(now time.Time, badges map[string]string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	for _, a := range u.apps {
		n := badgeCount(badges[a.dock])
		if n > a.badge && u.primed {
			a.lastAt = now
		}
		a.badge = n
	}
	u.primed = true
	svg := unreadSVG(u.waiting(), now)
	if svg != u.shown {
		u.shown = svg
		u.vars.set("unread_svg", svg)
	}
}

func (u *unread) waiting() []*unreadApp {
	var out []*unreadApp
	for _, a := range u.apps {
		if a.badge > 0 {
			out = append(out, a)
		}
	}
	return out
}

func capped(n, max int) string {
	if n > max {
		return fmt.Sprintf("%d+", max)
	}
	return strconv.Itoa(n)
}

func ago(at, now time.Time) string {
	d := now.Sub(at)
	switch {
	case at.IsZero():
		return ""
	case d < time.Hour:
		return capped(int(d.Minutes()), 59) + "m"
	case d < 24*time.Hour:
		return capped(int(d.Hours()), 23) + "h"
	default:
		return capped(int(d.Hours()/24), 9) + "d"
	}
}

func unreadSVG(apps []*unreadApp, now time.Time) string {
	if len(apps) == 0 {
		return dataURI(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">` + iconAt("eye", "#fff", 50, 50, 44) + `</svg>`)
	}
	var b strings.Builder
	step := 88.0 / float64(len(apps))
	for i, a := range apps {
		cx := 6 + step*(float64(i)+0.5)
		count := capped(a.badge, 99)
		fmt.Fprintf(&b, `<circle cx="%.1f" cy="40" r="20" fill="%s"/>`+
			`<text x="%.1f" y="46.5" text-anchor="middle" font-family="JetBrains Mono" font-size="18" fill="#000" stroke="#000" stroke-width="4" stroke-linejoin="round">%s</text>`+
			`<text x="%.1f" y="46.5" text-anchor="middle" font-family="JetBrains Mono" font-size="18" fill="#fff">%s</text>`+
			`<text x="%.1f" y="81" text-anchor="middle" font-family="JetBrains Mono" font-size="14" fill="%s">%s</text>`,
			cx, a.fill, cx, count, cx, count, cx, gray, ago(a.lastAt, now))
	}
	return dataURI(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">` + b.String() + `</svg>`)
}

func (u *unread) press() error {
	u.mu.Lock()
	apps := u.waiting()
	if len(apps) == 0 {
		u.mu.Unlock()
		return nil
	}
	a := apps[u.cursor%len(apps)]
	u.cursor++
	u.mu.Unlock()
	return unreadOpen(a.bundle)
}
