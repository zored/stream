package main

import (
	"cmp"
	"fmt"
	"html"
	"log"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	calBrowseFor = 5 * time.Second
	calPollEvery = time.Minute
	calHorizon   = 7 * 24 * time.Hour
	calSoon      = 5 * time.Minute
	amberRGB     = 0xFFD60A
	grayRGB      = 0x808080
	whiteRGB     = 0xFFFFFF
	greenRGB     = 0x00FF00
	soonBgRGB    = 0x0E4420
)

type calItem struct {
	id, ext, title string
	notes          string
	start, end     time.Time
	allDay         bool
	reminder       bool
	done           bool
}

type calendar struct {
	mu         sync.Mutex
	card       frame
	vars       *vars
	release    func()
	strip      func(uri string)
	covered    bool
	fetch      func(from, to time.Time) ([]calItem, error)
	items      []calItem
	browseTill time.Time
	pick       int
	knobDown   bool
	dismissed  string
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
	return slices.IndexFunc(items, func(it calItem) bool { return !it.allDay && !it.done })
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
		c.strip(calStripImage(c.items, c.pick))
		c.vars.set("cal_icon", calTileImage(c.items, c.pick))
		c.vars.set("cal_browse", "1")
	} else if c.covered {
		c.covered = false
		c.strip("")
		c.vars.set("cal_browse", "0")
	}
	var next *calItem
	bg := calBg(c.items, now, c.dismissed)
	key := calDate(now) + strconv.Itoa(int(bg))
	if i := calNext(c.items, now); i >= 0 {
		next = &c.items[i]
		key += "\n" + calWhen(*next, now) + " " + calTitle(*next)
	}
	c.vars.set("cal_led", strconv.Itoa(calLed(c.browsing())))
	c.vars.set("cal_card", c.card.render(key, func() string { return calCardImage(now, next, bg) }))
}

func calGlow(items []calItem, now time.Time) string {
	i := slices.IndexFunc(items, func(it calItem) bool {
		return !it.reminder && !it.allDay && it.end.After(now) && it.start.Sub(now) <= calSoon
	})
	if i < 0 {
		return ""
	}
	return items[i].id + "@" + items[i].start.String()
}

func calBg(items []calItem, now time.Time, dismissed string) uint32 {
	if g := calGlow(items, now); g != "" && g != dismissed {
		return soonBgRGB
	}
	return 0
}

func (c *calendar) dismissLocked() {
	if g := calGlow(c.items, time.Now()); g != "" {
		c.dismissed = g
	}
}

func calLed(browsing bool) int {
	if browsing {
		return greenRGB
	}
	return whiteRGB
}

func (c *calendar) browsing() bool {
	return time.Now().Before(c.browseTill)
}

func (c *calendar) press() error {
	c.mu.Lock()
	c.dismissLocked()
	if c.browsing() {
		i := c.pick
		c.mu.Unlock()
		return c.act(i)
	}
	c.pick, c.browseTill = max(calNext(c.items, time.Now()), 0), time.Now().Add(calBrowseFor)
	go c.release()
	c.publishLocked()
	c.mu.Unlock()
	return nil
}

func (c *calendar) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.browseTill = time.Time{}
	c.publishLocked()
}

func (c *calendar) capturing() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.browsing()
}

func (c *calendar) turn(direction int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.browsing() {
		return false
	}
	c.dismissLocked()
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
	c.dismissLocked()
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
	return calImageBg(w, h, rotate, 0, texts...)
}

func calImageBg(w, h float64, rotate bool, bg uint32, texts ...calText) string {
	uri, err := calPNG(w, h, rotate, bg, texts)
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

var calMonths = [...]string{"янв", "фев", "мар", "апр", "мая", "июн", "июл", "авг", "сен", "окт", "ноя", "дек"}

func calDate(t time.Time) string {
	return fmt.Sprintf("%d %s", t.Day(), calMonths[t.Month()-1])
}

func calCardImage(now time.Time, it *calItem, bg uint32) string {
	texts := []calText{{text: calDate(now), y: 68, w: 88, h: 20, size: captionSize, rgb: whiteRGB, center: true}}
	if it != nil {
		texts = append(texts, calText{text: calTitle(*it), tail: " " + calWhen(*it, now), w: 88, h: 64, size: 15, rgb: whiteRGB, tailRGB: grayRGB})
	}
	return calImageBg(88, 88, false, bg, texts...)
}

var calWeekdays = [...]string{"вс", "пн", "вт", "ср", "чт", "пт", "сб"}

func calStripImage(items []calItem, pick int) string {
	const w, h = stripDrawH, stripDrawW
	if pick >= len(items) {
		return calImage(w, h, true, calText{text: "·", y: 12, w: w, h: 24, size: 16, rgb: grayRGB, center: true})
	}
	it := items[pick]
	tail := calNotes(it.notes)
	if tail != "" {
		tail = " " + tail
	}
	return calImage(w, h, true, calText{text: calTitle(it), tail: tail, x: 4, y: 3, w: w - 8, h: 44, size: 15, rgb: whiteRGB, tailRGB: grayRGB})
}

var (
	calLink  = regexp.MustCompile(`<?(?:https?://|www\.)[^\s<>()\[\]]+>?`)
	calEmpty = regexp.MustCompile(`\(\s*\)|\[\s*\]`)
	calBreak = regexp.MustCompile(`(?i)<br\s*/?>|</p>|</div>|</li>`)
	calTag   = regexp.MustCompile(`<[^>]*>`)
	calJunk  = regexp.MustCompile(`(?im)^\s*(?:[-=_*~]{3,}|(?:id звонка|код доступа|пароль|meeting id|passcode)\s*:.*)\s*$`)
)

func calNotes(s string) string {
	s = html.UnescapeString(calTag.ReplaceAllString(calBreak.ReplaceAllString(s, "\n"), ""))
	return strings.Join(strings.Fields(calEmpty.ReplaceAllString(calLink.ReplaceAllString(calJunk.ReplaceAllString(s, ""), ""), " ")), " ")
}

func calTileImage(items []calItem, pick int) string {
	if pick >= len(items) {
		return calImage(88, 88, false, calText{text: "·", y: 33, w: 88, h: 22, size: 16, rgb: grayRGB, center: true})
	}
	it := items[pick]
	when := calWeekdays[it.start.Weekday()]
	if !it.allDay {
		when += " " + it.start.Format("15:04")
	}
	texts := []calText{
		{text: calDate(it.start), w: 88, h: 22, size: 16, rgb: whiteRGB},
		{text: when, y: 24, w: 88, h: 22, size: 16, rgb: cmp.Or(calColor(it, time.Now()), whiteRGB)},
	}
	if it.reminder {
		box := "○"
		if it.done {
			box = "●"
		}
		texts = append(texts, calText{text: box, y: 52, w: 88, h: 34, size: 28, rgb: whiteRGB})
	}
	return calImage(88, 88, false, texts...)
}
