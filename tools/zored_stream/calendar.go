package main

import (
	"cmp"
	"errors"
	"fmt"
	"html"
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
	calLineRune  = 9
	calTitleRune = 9
	calRows      = 4
	amber        = "#FFB000"
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
	return slices.IndexFunc(items, func(it calItem) bool { return !it.allDay && !it.start.After(now.Add(calAhead)) })
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
		c.vars.set("cal_svg", calBrowseSVG(c.items, c.pick, now))
		return
	}
	c.vars.set("cal_browse", "0")
	line := ""
	if i := calNext(c.items, now); i >= 0 {
		line = calLineSVG(c.items[i], now)
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
	c.browseTill = time.Time{}
	if it.reminder {
		c.items[i].done = true
	}
	c.publishLocked()
	c.mu.Unlock()
	var err error
	if it.reminder {
		log.Printf("calendar: complete %q", it.title)
		err = calComplete(it.id)
	} else {
		log.Printf("calendar: open %q", it.title)
		err = calOpen(it)
	}
	go c.poll()
	return err
}

const calFont = `font-family="JetBrains Mono, Apple Color Emoji"`

func calTitle(s string) string {
	s = strings.ToLower(strings.Join(strings.Fields(s), " "))
	if s == "" {
		return "·"
	}
	return s
}

func calCut(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return strings.TrimSpace(string(r[:n]))
	}
	return s
}

func calTime(it calItem) string {
	if it.allDay {
		return ""
	}
	return it.start.Format("15:04")
}

func calColor(it calItem, now time.Time) string {
	if it.done {
		return ""
	}
	if it.reminder && !it.allDay && it.start.Before(now) {
		return amber
	}
	if !it.reminder && it.start.After(now) && it.start.Sub(now) <= calSoon {
		return amber
	}
	return ""
}

func calGlyph(it calItem, x, y, r float64, color string) string {
	if !it.reminder {
		return ""
	}
	if it.done {
		return fmt.Sprintf(`<circle cx="%g" cy="%g" r="%g" fill="%s"/>`, x, y, r, color)
	}
	return fmt.Sprintf(`<circle cx="%g" cy="%g" r="%g" fill="none" stroke="%s" stroke-width="1.5"/>`, x, y, r-0.75, color)
}

func calLineSVG(it calItem, now time.Time) string {
	color := gray
	n := calLineRune
	if it.reminder {
		n -= 2
	}
	title := calCut(calTitle(it.title), n)
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 88 22">`)
	anchor, x := "middle", 44.0
	if it.reminder {
		start := 44 - (float64(len([]rune(title)))*16*0.6+12)/2
		b.WriteString(calGlyph(it, start+4.5, 11, 4.5, color))
		anchor, x = "start", start+12
	}
	fmt.Fprintf(&b, `<text x="%g" y="16.5" text-anchor="%s" %s font-size="16" fill="%s">%s</text>`, x, anchor, calFont, color,
		html.EscapeString(title))
	b.WriteString(`</svg>`)
	return dataURI(b.String())
}

func calWrap(s string, width, rows int) []string {
	var lines []string
	line := ""
	for _, w := range strings.Fields(s) {
		switch {
		case line == "":
			line = w
		case len([]rune(line))+1+len([]rune(w)) <= width:
			line += " " + w
		default:
			lines, line = append(lines, line), w
		}
	}
	lines = append(lines, line)
	if len(lines) > rows {
		lines = append(lines[:rows-1], strings.Join(lines[rows-1:], " "))
	}
	for i := range lines {
		lines[i] = calCut(lines[i], width)
	}
	return lines
}

func calBrowseSVG(items []calItem, pick int, now time.Time) string {
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">`)
	if pick >= len(items) {
		fmt.Fprintf(&b, `<circle cx="50" cy="50" r="4" fill="%s"/>`, gray)
		b.WriteString(`</svg>`)
		return dataURI(b.String())
	}
	it := items[pick]
	color := cmp.Or(calColor(it, now), "#fff")
	head := it.start.Format("02.01")
	if t := calTime(it); t != "" {
		head = t + " " + head
	}
	fmt.Fprintf(&b, `<text x="6" y="18" font-family="JetBrains Mono" font-size="12" fill="%s">%s</text>`, color, head)
	b.WriteString(calGlyph(it, 90, 14, 4.5, color))
	for i, line := range calWrap(calTitle(it.title), calTitleRune, calRows) {
		fmt.Fprintf(&b, `<text x="6" y="%d" %s font-size="16" fill="%s">%s</text>`, 42+i*17, calFont, gray, html.EscapeString(line))
	}
	b.WriteString(`</svg>`)
	return dataURI(b.String())
}
