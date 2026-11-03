package main

import (
	"cmp"
	"errors"
	"log"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	calHoldFor   = 400 * time.Millisecond
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
	fetch      func(from, to time.Time) ([]calItem, error)
	items      []calItem
	downAt     time.Time
	browseTill time.Time
	pick       int
	knobDown   bool
}

func newCalendar(v *vars, release func()) *calendar {
	return &calendar{vars: v, release: release, fetch: calFetch}
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
		c.vars.set("cal_browse", "1")
		c.vars.set("cal_svg", calBrowseImage(c.items, c.pick, now))
		return
	}
	c.vars.set("cal_browse", "0")
	line := ""
	if i := calNext(c.items, now); i >= 0 {
		line = calLineImage(c.items[i])
	}
	c.vars.set("cal_line", line)
}

func (c *calendar) browsing() bool {
	return time.Now().Before(c.browseTill)
}

func (c *calendar) down() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.downAt = time.Now()
	return nil
}

func (c *calendar) up() error {
	c.mu.Lock()
	if c.downAt.IsZero() {
		c.mu.Unlock()
		return errors.New("release without press")
	}
	held := time.Since(c.downAt)
	c.downAt = time.Time{}
	if held >= calHoldFor {
		i := c.pick
		if !c.browsing() {
			i = calNext(c.items, time.Now())
		}
		c.mu.Unlock()
		return c.act(i)
	}
	if c.browsing() {
		c.browseTill = time.Time{}
	} else {
		c.pick, c.browseTill = 0, time.Now().Add(calBrowseFor)
		go c.release()
	}
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

func calImage(w, h float64, texts ...calText) string {
	uri, err := calPNG(w, h, texts)
	if err != nil {
		log.Print(err)
	}
	return uri
}

func calLineImage(it calItem) string {
	return calImage(88, 24, calText{text: calTitle(it), y: 0, w: 88, h: 24, size: 16, rgb: grayRGB, center: true})
}

func calBrowseImage(items []calItem, pick int, now time.Time) string {
	if pick >= len(items) {
		return calImage(100, 100, calText{text: "·", y: 40, w: 100, h: 20, size: 16, rgb: grayRGB, center: true})
	}
	it := items[pick]
	return calImage(100, 100,
		calText{text: calTime(it) + it.start.Format("02.01"), x: 6, y: 6, w: 88, h: 17, size: 13, rgb: cmp.Or(calColor(it, now), whiteRGB)},
		calText{text: calTitle(it), x: 6, y: 27, w: 88, h: 67, size: 16, rgb: grayRGB})
}
