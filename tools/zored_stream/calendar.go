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
	"unicode"
)

const (
	calHoldFor   = 400 * time.Millisecond
	calBrowseFor = 5 * time.Second
	calPollEvery = time.Minute
	calHorizon   = 7 * 24 * time.Hour
	calAhead     = 12 * time.Hour
	calSoon      = 5 * time.Minute
	calLineRune  = 11
	calTitleRune = 11
	calRows      = 3
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
	return items
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

func calTitle(s string) string {
	s = strings.Join(strings.FieldsFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || !(unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsPunct(r) || r == '+' || r == '=')
	}), " ")
	if s == "" {
		return "·"
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
	color := cmp.Or(calColor(it, now), gray)
	x, n := 0.0, calLineRune
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 88 17">`)
	if it.reminder {
		b.WriteString(calGlyph(it, 5, 9, 4, color))
		x, n = 13, calLineRune-1
	}
	fmt.Fprintf(&b, `<text x="%g" y="13.5" font-family="JetBrains Mono" font-size="13" fill="%s">%s</text>`, x, color,
		html.EscapeString(clip(calTitle(it.title), n)))
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
		lines = append(lines[:rows-1], strings.Join(lines[rows-1:], " ")+"")
	}
	for i := range lines {
		lines[i] = clip(lines[i], width)
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
	x := 6.0
	if it.reminder {
		b.WriteString(calGlyph(it, 12, 17, 6, color))
		x = 24
	}
	if t := calTime(it); t != "" {
		fmt.Fprintf(&b, `<text x="%g" y="24" font-family="JetBrains Mono" font-size="20" fill="%s">%s</text>`, x, color, t)
	}
	for i, line := range calWrap(calTitle(it.title), calTitleRune, calRows) {
		fmt.Fprintf(&b, `<text x="6" y="%d" font-family="JetBrains Mono" font-size="13" fill="%s">%s</text>`, 44+i*15, gray, html.EscapeString(line))
	}
	fmt.Fprintf(&b, `<text x="6" y="92" font-family="JetBrains Mono" font-size="13" fill="%s">%s</text>`, gray, it.start.Format("02.01"))
	b.WriteString(`</svg>`)
	return dataURI(b.String())
}
