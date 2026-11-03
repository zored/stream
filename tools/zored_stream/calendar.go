package main

import (
	"cmp"
	"fmt"
	"log"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	calBrowseFor = 5 * time.Second
	calPollEvery = time.Minute
	calHorizon   = 7 * 24 * time.Hour
	calAhead     = 12 * time.Hour
	calSoon      = 5 * time.Minute
	amberRGB     = 0xFFB000
	grayRGB      = 0x808080
	whiteRGB     = 0xFFFFFF
)

type calItem struct {
	id, ext, title string
	start, end     time.Time
	allDay         bool
	reminder       bool
	done           bool
}

type calendar struct {
	mu         sync.Mutex
	vars       *vars
	release    func()
	strip      func(uri string)
	covered    bool
	fetch      func(from, to time.Time) ([]calItem, error)
	items      []calItem
	browseTill time.Time
	pick       int
	knobDown   bool
}

func newCalendar(v *vars, release func(), strip func(string)) *calendar {
	return &calendar{vars: v, release: release, strip: strip, fetch: calFetch}
}

func (c *calendar) poll() {
	now := time.Now()
	items, err := c.fetch(now.Add(-calHorizon), now.Add(calHorizon))
	if err != nil {
		log.Printf("calendar: %v", err)
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = calKeep(items, now)
	c.pick = min(c.pick, max(len(c.items)-1, 0))
	c.publishLocked()
}

func calKeep(items []calItem, now time.Time) []calItem {
	items = slices.DeleteFunc(slices.Clone(items), func(it calItem) bool { return !it.reminder && !it.end.After(now) })
	slices.SortStableFunc(items, func(a, b calItem) int { return a.start.Compare(b.start) })
	seen := map[string]bool{}
	return slices.DeleteFunc(items, func(it calItem) bool {
		dup := seen[it.id]
		seen[it.id] = true
		return dup
	})
}

func calNext(items []calItem, now time.Time) int {
	return slices.IndexFunc(items, func(it calItem) bool { return !it.allDay && !it.done && !it.start.After(now.Add(calAhead)) })
}

func (c *calendar) publish() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.publishLocked()
}

func (c *calendar) publishLocked() {
	now := time.Now()
	if c.browsing() {
		c.covered = true
		c.strip(calStripImage(c.items, c.pick, now))
		c.vars.set("cal_icon", calIcon(c.items, c.pick))
		c.vars.set("cal_browse", "1")
	} else if c.covered {
		c.covered = false
		c.strip("")
		c.vars.set("cal_browse", "0")
	}
	line := ""
	if i := calNext(c.items, now); i >= 0 {
		line = calCardImage(c.items[i], now)
	}
	c.vars.set("cal_card", line)
}

func (c *calendar) browsing() bool {
	return time.Now().Before(c.browseTill)
}

func (c *calendar) press() error {
	c.mu.Lock()
	if c.browsing() {
		i := c.pick
		c.mu.Unlock()
		return c.act(i)
	}
	c.pick, c.browseTill = 0, time.Now().Add(calBrowseFor)
	go c.release()
	c.publishLocked()
	c.mu.Unlock()
	return nil
}

func (c *calendar) turn(direction int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.browsing() {
		return false
	}
	if n := len(c.items); n > 0 {
		c.pick = min(max(c.pick+direction, 0), n-1)
	}
	c.browseTill = time.Now().Add(calBrowseFor)
	c.publishLocked()
	return true
}

func (c *calendar) knob(down bool) (bool, error) {
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

func (c *calendar) act(i int) error {
	c.mu.Lock()
	if i < 0 || i >= len(c.items) {
		c.mu.Unlock()
		return nil
	}
	it := c.items[i]
	if it.reminder {
		c.items[i].done = !it.done
		if c.browsing() {
			c.browseTill = time.Now().Add(calBrowseFor)
		}
	} else {
		c.browseTill = time.Time{}
	}
	c.publishLocked()
	c.mu.Unlock()
	var err error
	if it.reminder {
		log.Printf("calendar: done=%v %q", !it.done, it.title)
		err = calSetDone(it.id, !it.done)
	} else {
		log.Printf("calendar: open %q", it.title)
		err = calOpen(it)
	}
	go c.poll()
	return err
}

func calTitle(it calItem) string {
	s := strings.ToLower(strings.Join(strings.Fields(it.title), " "))
	if s == "" {
		s = "·"
	}
	switch {
	case !it.reminder:
		return s
	case it.done:
		return "● " + s
	default:
		return "○ " + s
	}
}

func calTime(it calItem) string {
	if it.allDay {
		return ""
	}
	return it.start.Format("15:04 ")
}

func calColor(it calItem, now time.Time) uint32 {
	if it.done {
		return 0
	}
	if it.reminder && !it.allDay && it.start.Before(now) {
		return amberRGB
	}
	if !it.reminder && it.start.After(now) && it.start.Sub(now) <= calSoon {
		return amberRGB
	}
	return 0
}

func calImage(w, h float64, rotate bool, texts ...calText) string {
	uri, err := calPNG(w, h, rotate, texts)
	if err != nil {
		log.Print(err)
	}
	return uri
}

func calIn(d time.Duration) string {
	sign := "+"
	if d < 0 {
		sign, d = "-", -d
	}
	switch {
	case d >= 24*time.Hour:
		return fmt.Sprintf("%s%dd", sign, int(d.Round(time.Hour).Hours()/24))
	case d >= time.Hour:
		return fmt.Sprintf("%s%dh", sign, int(d.Round(time.Minute).Minutes()/60))
	default:
		return fmt.Sprintf("%s%dm", sign, int(d.Minutes()))
	}
}

func calWhen(it calItem, now time.Time) string {
	if !it.reminder && !it.start.After(now) {
		return strings.TrimPrefix(calIn(it.end.Sub(now)), "+")
	}
	return calIn(it.start.Sub(now))
}

func calCardImage(it calItem, now time.Time) string {
	return calImage(88, 66, false,
		calText{text: calTime(it) + calWhen(it, now), w: 88, h: 22, size: 16, rgb: whiteRGB, center: true},
		calText{text: calTitle(it), y: 24, w: 88, h: 42, size: 15, rgb: grayRGB, center: true})
}

var calWeekdays = [...]string{"вс", "пн", "вт", "ср", "чт", "пт", "сб"}

func calStripImage(items []calItem, pick int, now time.Time) string {
	const w, h = stripDrawH, stripDrawW
	if pick >= len(items) {
		return calImage(w, h, true, calText{text: "·", y: 12, w: w, h: 24, size: 16, rgb: grayRGB, center: true})
	}
	it := items[pick]
	when := calTime(it)
	if y, m, d := it.start.Date(); y != now.Year() || m != now.Month() || d != now.Day() {
		when = calWeekdays[it.start.Weekday()] + " " + it.start.Format("02.01") + " " + when
	}
	pad := strings.Repeat(" ", len([]rune(when)))
	return calImage(w, h, true,
		calText{text: when, x: 4, y: 3, w: w - 8, h: 22, size: 15, rgb: cmp.Or(calColor(it, now), grayRGB)},
		calText{text: pad + calTitle(it), x: 4, y: 3, w: w - 8, h: 44, size: 15, rgb: whiteRGB})
}

func calIcon(items []calItem, pick int) string {
	var body string
	switch {
	case pick >= len(items):
		body = fmt.Sprintf(`<circle cx="50" cy="50" r="5" fill="%s"/>`, gray)
	case !items[pick].reminder:
		body = `<rect x="18" y="24" width="64" height="58" rx="8" fill="none" stroke="#fff" stroke-width="6"/>` +
			`<path d="M18 42h64M36 16v16M64 16v16" stroke="#fff" stroke-width="6" stroke-linecap="round"/>`
	case items[pick].done:
		body = `<rect x="20" y="20" width="60" height="60" rx="10" fill="#fff"/>` +
			`<path d="M34 51l11 11 21-24" fill="none" stroke="#000" stroke-width="8" stroke-linecap="round" stroke-linejoin="round"/>`
	default:
		body = `<rect x="23" y="23" width="54" height="54" rx="8" fill="none" stroke="#fff" stroke-width="6"/>`
	}
	return dataURI(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">` + body + `</svg>`)
}
