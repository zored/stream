package main

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	chatsBrowseFor  = 5 * time.Second
	chatsOnTile     = 4
	chatsRetry      = 30 * time.Second
	chatsLedIdle    = 0xFFFFFF
	chatsLedBrowse  = 0x00FF00
	chatsMutedAlpha = 0.4
	chatsHoldFor    = 400 * time.Millisecond
	chatsReadFor    = 10 * time.Second
	chatsMsgsMax    = 50
	chatsTextSize   = 12
	chatsLineH      = chatsTextSize * 1.25
	chatsTextView   = 46
	chatsPushTurns  = 3
	chatsPushIdle   = 1500 * time.Millisecond
	chatsBarH       = 2.0
)

type chatMsg struct {
	text   string
	sender []byte
	photos []chatPhoto
}

type chatEntry struct {
	app    string
	id     string
	title  string
	unread int
	at     time.Time
	text   string
	avatar []byte
	sender []byte
	open   string
	muted  bool
	msgs   []chatMsg
	read   func(context.Context) error
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
	slideGen   int
	sliding    bool
	msg        int
	scrollTo   float64
	scrollAt   float64
	kick       chan struct{}
	holdGen    int
	holding    bool
	held       bool
	hidden     map[string]time.Time
	photo      int
	photos     *photos
	blocked    func() bool
	push       int
	pushTo     float64
	pushAt     float64
	pushGen    int
}

func newChats(v *vars, release func(), strip func(string)) *chats {
	return &chats{vars: v, release: release, strip: strip, byApp: map[string][]chatEntry{}, opener: launchURL,
		kick: make(chan struct{}, 1), hidden: map[string]time.Time{}, photo: -1, photos: newPhotos(), blocked: func() bool { return false }}
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
	c.mergeLocked()
	c.publishLocked()
}

func chatsKey(e chatEntry) string {
	return e.app + "/" + e.id
}

// mergeLocked hides chats marked read here until their source drops them or a newer message comes.
func (c *chats) mergeLocked() {
	live := map[string]bool{}
	var out []chatEntry
	for _, e := range chatsMerge(c.byApp) {
		k := chatsKey(e)
		live[k] = true
		if at, ok := c.hidden[k]; ok {
			if !e.at.After(at) {
				continue
			}
			delete(c.hidden, k)
		}
		out = append(out, e)
	}
	for k := range c.hidden {
		if !live[k] {
			delete(c.hidden, k)
		}
	}
	c.list = out
	c.pick = min(c.pick, max(len(c.list)-1, 0))
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
	slices.SortStableFunc(out, func(a, b chatEntry) int {
		if a.muted != b.muted {
			if a.muted {
				return 1
			}
			return -1
		}
		return b.at.Compare(a.at)
	})
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
	c.photoLocked()
	now := time.Now()
	var svg string
	if c.sliding {
		return
	}
	if c.browsing() {
		c.covered = true
		svg = chatsPickImage(c.list, c.pick, c.msg, now)
		c.strip(c.stripLocked())
	} else {
		if c.covered {
			c.covered = false
			c.strip("")
		}
		svg = chatsTileImage(c.list[:min(len(c.list), chatsOnTile)], len(c.list) > chatsOnTile, now)
	}
	c.vars.set("unread_svg", svg)
}

func (c *chats) stripLocked() string {
	if c.pick >= len(c.list) {
		return chatsTextStrip("·", 0, 0)
	}
	return chatsTextStrip(chatsView(c.list[c.pick], c.msg).text, c.scrollAt, c.pushAt)
}

// down arms the hold: 400 ms held while browsing marks the picked chat read and eats the release.
func (c *chats) down() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.holdGen++
	c.held = false
	c.holding = c.browsing()
	if c.holding {
		gen := c.holdGen
		time.AfterFunc(chatsHoldFor, func() { c.holdRead(gen) })
	}
	return nil
}

func (c *chats) up() error {
	c.mu.Lock()
	held := c.held
	c.holdGen++
	c.holding, c.held = false, false
	c.mu.Unlock()
	if held {
		return nil
	}
	return c.press()
}

func (c *chats) holdRead(gen int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if gen != c.holdGen || !c.holding || !c.browsing() {
		return
	}
	c.held = true
	c.markReadLocked()
}

func (c *chats) markReadLocked() {
	if c.pick >= len(c.list) {
		return
	}
	e := c.list[c.pick]
	k := chatsKey(e)
	if e.read == nil {
		log.Printf("chats: %s has no read call", k)
		return
	}
	c.hidden[k] = e.at
	c.mergeLocked()
	c.resetMsgLocked()
	if len(c.list) == 0 {
		c.browseTill = time.Time{}
	} else {
		c.browseTill = time.Now().Add(chatsBrowseFor)
	}
	c.publishLocked()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), chatsReadFor)
		defer cancel()
		if err := e.read(ctx); err != nil {
			log.Printf("chats: mark %s read: %v", k, err)
			c.mu.Lock()
			delete(c.hidden, k)
			c.mergeLocked()
			c.publishLocked()
			c.mu.Unlock()
			return
		}
		log.Printf("chats: marked %s read", k)
	}()
}

func (c *chats) resetMsgLocked() {
	c.msg = 0
	c.resetScrollLocked()
}

func (c *chats) resetScrollLocked() {
	c.scrollTo, c.scrollAt, c.photo = 0, 0, -1
	c.push, c.pushTo, c.pushAt = 0, 0, 0
	c.pushGen++
}

func (c *chats) msgCount() int {
	return max(len(c.list[c.pick].msgs), 1)
}

// moveMsgLocked walks the messages through the chats in a loop: past the last message comes the next chat's first, before the first the previous chat's last; toEnd lands at the end of the text.
func (c *chats) moveMsgLocked(direction int, toEnd bool) {
	to := c.msg + direction
	switch n := c.msgCount(); {
	case to >= 0 && to < n:
		c.msg = to
		c.resetScrollLocked()
	case len(c.list) == 1:
		c.msg = (to%n + n) % n
		c.resetScrollLocked()
	default:
		c.stepLocked(direction, direction < 0, toEnd)
		return
	}
	if toEnd {
		c.scrollTo = chatsScrollEnd(chatsView(c.list[c.pick], c.msg).text)
		c.scrollAt = c.scrollTo
	}
	c.publishLocked()
}

func (c *chats) kickLocked() {
	select {
	case c.kick <- struct{}{}:
	default:
	}
}

// pushLocked counts the detents past an end of the shown message; the bar grows on each, the last one moves on.
func (c *chats) pushLocked(direction int) {
	if c.push*direction < 0 {
		c.push = 0
	}
	c.push += direction
	if c.push*direction >= chatsPushTurns {
		c.moveMsgLocked(direction, direction < 0)
		return
	}
	c.pushTo = float64(c.push) / chatsPushTurns
	c.pushGen++
	gen := c.pushGen
	time.AfterFunc(chatsPushIdle, func() { c.pushIdle(gen) })
	c.kickLocked()
}

func (c *chats) pushIdle(gen int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if gen != c.pushGen {
		return
	}
	c.push, c.pushTo = 0, 0
	c.kickLocked()
}

func (c *chats) photosLocked() []chatPhoto {
	if c.pick >= len(c.list) {
		return nil
	}
	msgs := c.list[c.pick].msgs
	if len(msgs) == 0 {
		return nil
	}
	return msgs[min(max(c.msg, 0), len(msgs)-1)].photos
}

// photoLocked publishes the photo the fork spreads over the tiles; the shown message's photos load ahead while browsing.
func (c *chats) photoLocked() {
	if !c.browsing() || c.blocked() {
		c.photos.show("")
		return
	}
	ps := c.photosLocked()
	for _, ph := range ps {
		c.photos.load(ph, c.publish)
	}
	if c.photo < 0 || c.photo >= len(ps) || !c.photos.cached(ps[c.photo].key) {
		c.photos.show("")
		return
	}
	c.photos.show(ps[c.photo].key)
}

// closePhoto: a tap on a tile under the photo goes back to the message text.
func (c *chats) closePhoto() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.photo < 0 {
		return nil
	}
	c.photo = -1
	if c.browsing() {
		c.browseTill = time.Now().Add(chatsBrowseFor)
	}
	c.publishLocked()
	return nil
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
	c.resetMsgLocked()
	go c.release()
	c.publishLocked()
	c.mu.Unlock()
	return nil
}

// knobTurn: a turn outside browse starts it on the first chat.
func (c *chats) knobTurn(direction int) error {
	if c.step(direction) {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.list) == 0 {
		return nil
	}
	c.pick, c.browseTill = 0, time.Now().Add(chatsBrowseFor)
	c.resetMsgLocked()
	c.publishLocked()
	return nil
}

// turn scrolls the shown message (крутилка справа, ряд 3) while browsing.
func (c *chats) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.browseTill = time.Time{}
	c.publishLocked()
}

func (c *chats) capturing() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.browsing()
}

func (c *chats) turn(direction int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.browsing() {
		return false
	}
	c.browseTill = time.Now().Add(chatsBrowseFor)
	if c.pick >= len(c.list) || c.sliding {
		return true
	}
	top := chatsScrollEnd(chatsView(c.list[c.pick], c.msg).text)
	n := len(c.photosLocked())
	atEnd := direction > 0 && (n > 0 && c.photo == n-1 || n == 0 && c.scrollTo >= top)
	atStart := direction < 0 && c.photo < 0 && c.scrollTo <= 0
	if atEnd || atStart {
		c.pushLocked(direction)
		return true
	}
	if c.push != 0 {
		c.push, c.pushTo = 0, 0
		c.pushGen++
		c.kickLocked()
	}
	if n > 0 && (c.photo >= 0 || direction > 0 && c.scrollTo >= top) {
		c.photo += direction
		c.publishLocked()
		return true
	}
	c.scrollTo = min(max(c.scrollTo+float64(direction)*chatsLineH/2, 0), top)
	c.kickLocked()
	return true
}

// msgTurn walks the unread messages of the picked chat (крутилка справа, ряд 2) while browsing.
func (c *chats) msgTurn(direction int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.browsing() {
		return false
	}
	c.browseTill = time.Now().Add(chatsBrowseFor)
	if c.pick >= len(c.list) || c.sliding {
		return true
	}
	c.moveMsgLocked(direction, false)
	return true
}

func (c *chats) animate() {
	for range c.kick {
		ticker := time.NewTicker(time.Second / animationHz)
		for c.scrollStep() {
			<-ticker.C
		}
		ticker.Stop()
	}
}

func (c *chats) scrollStep() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.browsing() || c.sliding {
		return false
	}
	moving := ease(&c.scrollAt, c.scrollTo, 0.5)
	moving = ease(&c.pushAt, c.pushTo, 0.02) || moving
	c.strip(c.stripLocked())
	return moving
}

// step moves to the next or previous chat while browsing, in a loop.
func (c *chats) step(direction int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.browsing() {
		return false
	}
	c.browseTill = time.Now().Add(chatsBrowseFor)
	c.stepLocked(direction, false, false)
	return true
}

// stepLocked lands on the first message of the next or previous chat, or on its last one with last, scrolled to the end with toEnd.
func (c *chats) stepLocked(direction int, last, toEnd bool) {
	n := len(c.list)
	if n == 0 {
		c.publishLocked()
		return
	}
	from, fromMsg := c.pick, c.msg
	c.pick = ((c.pick+direction)%n + n) % n
	if c.pick == from {
		c.publishLocked()
		return
	}
	c.resetMsgLocked()
	if last {
		c.msg = c.msgCount() - 1
	}
	if toEnd {
		c.scrollTo = chatsScrollEnd(chatsView(c.list[c.pick], c.msg).text)
		c.scrollAt = c.scrollTo
	}
	c.slideGen++
	c.sliding = true
	c.strip(c.stripLocked())
	go c.slide(c.slideGen, chatsView(c.list[from], fromMsg), chatsView(c.list[c.pick], c.msg), direction)
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
			img := chatsSlideImage(from, to, 1-(1-t)*(1-t), direction, time.Now())
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
	if e.open == "" {
		return fmt.Errorf("chats: %s %s has no url to open", e.app, e.id)
	}
	start := time.Now()
	err := c.opener(e.open)
	log.Printf("chats: open %s %s in %s", e.app, e.id, time.Since(start).Round(time.Millisecond))
	return err
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
	chatsRed     = redRGB
	chatsDark    = 0x333333
	chatsBlack   = 0x1C1C1E
	chatsFrames  = 6
	chatsFrameAt = 25 * time.Millisecond
)

func chatsLabel(text string, cx, cy, size float64, rgb uint32) calText {
	lh := size * 1.25
	return calText{text: text, x: cx - 30, y: cy - lh/2, w: 60, h: lh, size: size, rgb: rgb, center: true}
}

func chatsBadges(e chatEntry, x, y, w, r, size float64, now time.Time) ([]chatsDisc, []calText) {
	count := uint32(chatsRed)
	if e.muted {
		count = grayRGB
	}
	cx, top, bottom := x+w-r, y+r, y+w-r
	discs := []chatsDisc{{cx: cx, cy: bottom, r: r, rgb: chatsDark}}
	texts := []calText{chatsLabel(ago(e.at, now), cx, bottom, size*0.85, whiteRGB)}
	if e.unread > 0 {
		discs = append(discs, chatsDisc{cx: cx, cy: top, r: r, rgb: count})
		texts = append(texts, chatsLabel(capped(e.unread, 99), cx, top, size, whiteRGB))
	}
	return discs, texts
}

// chatsView is the browse card of one message: its text and sender, the red badge counts the messages after it.
func chatsView(e chatEntry, msg int) chatEntry {
	if len(e.msgs) == 0 {
		return e
	}
	msg = min(max(msg, 0), len(e.msgs)-1)
	m := e.msgs[msg]
	e.text, e.sender, e.unread = m.text, m.sender, len(e.msgs)-1-msg
	return e
}

func chatsSender(e chatEntry, x, y, w, r float64) []chatsDisc {
	if len(e.sender) == 0 {
		return nil
	}
	return []chatsDisc{{avatar: e.sender, cx: x + r, cy: y + w - r, r: r}}
}

func chatsFace(e chatEntry, cx, cy, r float64, square bool) (chatsDisc, []calText) {
	d := chatsDisc{avatar: e.avatar, cx: cx, cy: cy, r: r, square: square}
	if len(e.avatar) > 0 {
		return d, nil
	}
	d.rgb = chatsBlack
	n := 2
	if r >= 40 {
		n = 3
	}
	size := 2 * r * 0.8 / float64(n) / 1.25
	lines := chatsLines(e.title, n, int(2*r*0.9/(size*0.6)))
	top := cy - float64(len(lines)-1)*size*1.25/2
	texts := make([]calText, len(lines))
	for i, l := range lines {
		texts[i] = chatsLabel(l, cx, top+float64(i)*size*1.25, size, whiteRGB)
		texts[i].x, texts[i].w = cx-r, 2*r
	}
	return d, texts
}

func chatsLines(title string, n, width int) []string {
	var lines []string
	for _, w := range strings.FieldsFunc(strings.ToLower(title), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len(lines) == n {
			break
		}
		if rs := []rune(w); len(rs) > width {
			w = string(rs[:width])
		}
		lines = append(lines, w)
	}
	if len(lines) == 0 {
		return []string{chatsInitial(title)}
	}
	return lines
}

func chatsTileImage(list []chatEntry, more bool, now time.Time) string {
	if len(list) == 0 {
		return chatsNoneImage()
	}
	layer := func(e chatEntry, d []chatsDisc, t []calText) chatsLayer {
		if e.muted {
			return chatsLayer{d, t, chatsMutedAlpha}
		}
		return chatsLayer{d, t, 1}
	}
	var layers []chatsLayer
	switch len(list) {
	case 1:
		d, t := chatsCard(list[0], 0, now)
		layers = append(layers, layer(list[0], d, t))
	case 2:
		for i, e := range list {
			o := 40 * float64(i)
			d, t := chatsFace(e, o+22, o+26, 21, true)
			bd, bt := chatsBadges(e, o, o, 48, 9, 9, now)
			layers = append(layers, layer(e, append(append([]chatsDisc{d}, chatsSender(e, o, o, 48, 9)...), bd...), append(t, bt...)))
		}
	default:
		grid := 88.0
		if more {
			grid = 76
			layers = append(layers, chatsLayer{nil, []calText{chatsLabel("+", 82, 44, 16, grayRGB)}, 1})
		}
		cell := grid / 2
		for i, e := range list {
			x, y := cell*float64(i%2), cell*float64(i/2)+(88-grid)/2
			d, t := chatsFace(e, x+cell*0.42, y+cell*0.58, cell*0.36, true)
			bd, bt := chatsBadges(e, x, y, cell, cell*0.2, cell*0.2, now)
			layers = append(layers, layer(e, append(append([]chatsDisc{d}, chatsSender(e, x, y, cell, cell*0.2)...), bd...), append(t, bt...)))
		}
	}
	return chatsLayersPNG(88, 88, layers)
}

func chatsCard(e chatEntry, dx float64, now time.Time) ([]chatsDisc, []calText) {
	d, t := chatsFace(e, 44+dx, 44, 44, true)
	bd, bt := chatsBadges(e, dx, 0, 88, 13, 13, now)
	return append(append([]chatsDisc{d}, chatsSender(e, dx, 0, 88, 13)...), bd...), append(t, bt...)
}

func chatsPickImage(list []chatEntry, pick, msg int, now time.Time) string {
	if pick >= len(list) {
		return chatsNoneImage()
	}
	d, t := chatsCard(chatsView(list[pick], msg), 0, now)
	return chatsPNG(88, 88, d, t)
}

func chatsSlideImage(from, to chatEntry, step float64, direction int, now time.Time) string {
	shift := 88 * float64(direction)
	d1, t1 := chatsCard(from, -shift*step, now)
	d2, t2 := chatsCard(to, shift*(1-step), now)
	return chatsPNG(88, 88, append(d1, d2...), append(t1, t2...))
}

func chatsNoneImage() string {
	return tileIcon("eye-closed", gray)
}

func chatsLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func chatsTextStrip(text string, offset, bar float64) string {
	const w, h = stripDrawH, stripDrawW
	text = chatsLine(text)
	if !utf8.ValidString(text) || text == "" {
		text = "·"
	}
	lines := chatsTextLines(text, w-8, chatsTextSize)
	uri, err := chatsStripPNG(w, h, calText{text: text, x: 4, y: 1 - offset, w: w - 8, h: max(float64(lines)*chatsLineH, chatsTextView), size: chatsTextSize, rgb: whiteRGB}, bar)
	if err != nil {
		log.Print(err)
	}
	return uri
}

// chatsScrollEnd is the last scroll offset of a message's text on the strip.
func chatsScrollEnd(text string) float64 {
	lines := chatsTextLines(chatsLine(text), stripDrawH-8, chatsTextSize)
	return max(float64(lines)*chatsLineH-chatsTextView, 0)
}
