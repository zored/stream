package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	orcaCLI      = "/Applications/Orca.app/Contents/Resources/bin/orca"
	orcaProfile  = "Library/Application Support/orca-dev-copy"
	orcaPoll     = 15 * time.Second
	orcaTimeout  = 10 * time.Second
	orcaTabWait  = 6 * time.Second
	orcaTabEvery = 100 * time.Millisecond
)

type orcaChat struct {
	id, name, text string
	model          string
	env            []string
	unread         bool
	at             time.Time
}

type orcaSpace struct {
	id, name, kind string
	env            []string
	active         bool
	chats          []orcaChat
}

func (s orcaSpace) at() time.Time {
	var at time.Time
	for _, c := range s.chats {
		if c.at.After(at) {
			at = c.at
		}
	}
	return at
}

func (s orcaSpace) unread() int {
	n := 0
	for _, c := range s.chats {
		if c.unread {
			n++
		}
	}
	return n
}

// entry is the chat card: its name, unread dot, age and icon.
func (c orcaChat) entry() chatEntry {
	unread := 0
	if c.unread {
		unread = 1
	}
	return chatEntry{app: "orca", id: c.id, title: c.name, unread: unread, at: c.at, sender: orcaIconPNG(c)}
}

var orcaModels = map[string]string{"claude": "claude", "codex": "openai"}

type orcaPS struct {
	Result struct {
		HostScope struct {
			OmittedHostSelectors []struct {
				Selector string `json:"selector"`
			} `json:"omittedHostSelectors"`
		} `json:"hostScope"`
		Worktrees []struct {
			WorktreeID    string `json:"worktreeId"`
			DisplayName   string `json:"displayName"`
			WorkspaceKind string `json:"workspaceKind"`
			IsArchived    bool   `json:"isArchived"`
			IsActive      bool   `json:"isActive"`
			Agents        []struct {
				OrcaSessionID        string `json:"orcaSessionId"`
				AgentType            string `json:"agentType"`
				ConversationName     string `json:"conversationName"`
				Prompt               string `json:"prompt"`
				LastAssistantMessage string `json:"lastAssistantMessage"`
				StateStartedAt       int64  `json:"stateStartedAt"`
				Activity             *struct {
					Listed bool   `json:"listed"`
					Unread bool   `json:"unread"`
					SortAt int64  `json:"sortAt"`
					Title  string `json:"title"`
				} `json:"activity"`
			} `json:"agents"`
		} `json:"worktrees"`
	} `json:"result"`
}

// orcaParse turns `orca worktree ps --json` of host env into workspaces with chats, freshest chat activity first on both levels, plus the selectors of the hosts it left out.
// Chats of a paired host carry no activity: all count as listed, read, at stateStartedAt.
func orcaParse(data []byte, env []string) ([]orcaSpace, [][]string, error) {
	var ps orcaPS
	if err := json.Unmarshal(data, &ps); err != nil {
		return nil, nil, fmt.Errorf("worktree ps %v: %w", env, err)
	}
	var more [][]string
	for _, h := range ps.Result.HostScope.OmittedHostSelectors {
		more = append(more, strings.Fields(h.Selector))
	}
	var spaces []orcaSpace
	for _, w := range ps.Result.Worktrees {
		if w.IsArchived {
			continue
		}
		s := orcaSpace{id: w.WorktreeID, name: w.DisplayName, kind: w.WorkspaceKind, env: env, active: w.IsActive}
		for _, a := range w.Agents {
			c := orcaChat{
				id:    a.OrcaSessionID,
				model: orcaModels[a.AgentType],
				name:  a.ConversationName,
				text:  cmp.Or(a.LastAssistantMessage, a.Prompt),
				env:   env,
				at:    time.UnixMilli(a.StateStartedAt),
			}
			if act := a.Activity; act != nil {
				if !act.Listed {
					continue
				}
				c.name, c.unread, c.at = cmp.Or(act.Title, c.name), act.Unread, time.UnixMilli(act.SortAt)
			}
			s.chats = append(s.chats, c)
		}
		if len(s.chats) == 0 {
			continue
		}
		slices.SortStableFunc(s.chats, func(a, b orcaChat) int { return b.at.Compare(a.at) })
		spaces = append(spaces, s)
	}
	orcaSort(spaces)
	return spaces, more, nil
}

func orcaSort(spaces []orcaSpace) {
	slices.SortStableFunc(spaces, func(a, b orcaSpace) int { return b.at().Compare(a.at()) })
}

// psAll reads the local host, then every paired host it names, in parallel.
func (o *orca) psAll() ([]orcaSpace, error) {
	out, err := o.run("worktree", "ps", "--json")
	if err != nil {
		return nil, err
	}
	spaces, more, err := orcaParse(out, nil)
	if err != nil {
		return nil, err
	}
	got := make([][]orcaSpace, len(more))
	errs := make([]error, len(more))
	var wg sync.WaitGroup
	for i, env := range more {
		wg.Go(func() {
			out, err := o.run(append([]string{"worktree", "ps"}, append(slices.Clone(env), "--json")...)...)
			if err == nil {
				got[i], _, err = orcaParse(out, env)
			}
			errs[i] = err
		})
	}
	wg.Wait()
	for _, g := range got {
		spaces = append(spaces, g...)
	}
	orcaSort(spaces)
	return spaces, errors.Join(errs...)
}

type orca struct {
	mu       sync.Mutex
	vars     *vars
	strip    func(uri string)
	run      func(args ...string) ([]byte, error)
	press    func(name string) error
	markRead func(name string) error
	closeTab func(name string) error
	front    func() error
	wake     chan struct{}
	spaces   []orcaSpace
	failed   bool
	space    string
	chat     int
	till     time.Time
	covered  bool
	knobDown bool
	shown    string
	full     map[string]string
	scrollTo float64
	scrollAt float64
	kick     chan struct{}
	push     int
	pushTo   float64
	pushAt   float64
	pushGen  int
}

const (
	orcaTitleH = 18.0
	orcaTextSz = 11.0
	orcaLineH  = orcaTextSz * 1.25
)

func newOrca(v *vars, strip func(string)) *orca {
	return &orca{vars: v, strip: strip, run: orcaRun, press: orcaPressRow, markRead: orcaMarkRead, closeTab: orcaCloseTab, front: orcaFront, wake: make(chan struct{}, 1),
		full: map[string]string{}, kick: make(chan struct{}, 1)}
}

func orcaRun(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), orcaTimeout)
	defer cancel()
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("orca: home dir: %w", err)
	}
	cmd := exec.CommandContext(ctx, orcaCLI, args...)
	cmd.Env = append(os.Environ(), "ORCA_USER_DATA_PATH="+filepath.Join(home, orcaProfile))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("orca %s: %w %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func (o *orca) poll() {
	var lastError string
	for {
		spaces, err := o.psAll()
		o.mu.Lock()
		o.failed = err != nil
		msg := ""
		if err != nil {
			msg = err.Error()
		}
		if msg != lastError && msg != "" {
			log.Printf("orca: %v", err)
		}
		lastError = msg
		if spaces != nil || err == nil {
			o.replaceLocked(spaces)
		}
		o.publishLocked()
		o.mu.Unlock()
		select {
		case <-o.wake:
		case <-time.After(orcaPoll):
		}
	}
}

// replaceLocked takes a fresh poll; while browsing the shown chat stays put though the order changed, a gone chat closes the browse.
func (o *orca) replaceLocked(spaces []orcaSpace) {
	if !o.browsing() {
		o.spaces = spaces
		return
	}
	id := o.shownLocked().id
	o.spaces = spaces
	i := o.index()
	j := -1
	if i >= 0 {
		j = slices.IndexFunc(spaces[i].chats, func(c orcaChat) bool { return c.id == id })
	}
	if j < 0 {
		o.closeLocked()
		return
	}
	o.chat = j
}

func (o *orca) refresh() {
	select {
	case o.wake <- struct{}{}:
	default:
	}
}

func (o *orca) index() int {
	return slices.IndexFunc(o.spaces, func(s orcaSpace) bool { return s.id == o.space })
}

func (o *orca) browsing() bool {
	return o.space != "" && time.Now().Before(o.till)
}

func (o *orca) capturing() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.browsing()
}

func (o *orca) close() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.closeLocked()
	o.publishLocked()
}

func (o *orca) closeLocked() {
	o.space, o.chat, o.knobDown = "", 0, false
}

func (o *orca) expire() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.space != "" && !o.browsing() {
		o.closeLocked()
		o.publishLocked()
	}
}

// step walks the workspaces (крутилка справа, ряд 2); the first detent opens the browse.
func (o *orca) step(d int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.spaces) == 0 {
		return
	}
	i := -1
	if o.browsing() {
		i = o.index()
	}
	o.space, o.chat = o.spaces[agentStep(i, d, len(o.spaces))].id, 0
	o.till = time.Now().Add(chatsBrowseFor)
	o.publishLocked()
	if i < 0 {
		o.refresh()
	}
}

// chatStep walks all chats by activity across workspaces (крутилка справа, ряд 3), only while browsing.
func (o *orca) chatStep(d int) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.browsing() {
		return false
	}
	o.till = time.Now().Add(chatsBrowseFor)
	o.moveChatLocked(d, false)
	return true
}

type orcaSlot struct{ space, chat int }

// orcaFlat is every chat of every workspace, freshest activity first.
func orcaFlat(spaces []orcaSpace) []orcaSlot {
	var flat []orcaSlot
	for i, s := range spaces {
		for j := range s.chats {
			flat = append(flat, orcaSlot{i, j})
		}
	}
	slices.SortStableFunc(flat, func(a, b orcaSlot) int {
		return spaces[b.space].chats[b.chat].at.Compare(spaces[a.space].chats[a.chat].at)
	})
	return flat
}

// moveChatLocked walks all chats across workspaces by activity, freshest first, in a loop; toEnd lands at the end of the reply.
func (o *orca) moveChatLocked(d int, toEnd bool) {
	i := o.index()
	flat := orcaFlat(o.spaces)
	at := slices.Index(flat, orcaSlot{i, min(o.chat, len(o.spaces[i].chats)-1)})
	next := flat[(at+d%len(flat)+len(flat))%len(flat)]
	o.space, o.chat = o.spaces[next.space].id, next.chat
	o.shown = ""
	o.publishLocked()
	if toEnd {
		o.scrollTo = o.scrollEndLocked()
		o.scrollAt = o.scrollTo
		o.publishLocked()
	}
}

func (o *orca) scrollEndLocked() float64 {
	lines := orcaRichLines(o.textLocked(o.shownLocked()), stripDrawH-8, orcaTextSz)
	return max(orcaTitleH+float64(lines)*orcaLineH-stripDrawW, 0)
}

// pushLocked counts the detents past an end of the reply; the bar grows on each, the last one moves to the next or previous chat.
func (o *orca) pushLocked(d int) {
	if o.push*d < 0 {
		o.push = 0
	}
	o.push += d
	if o.push*d >= chatsPushTurns {
		o.moveChatLocked(d, d < 0)
		return
	}
	o.pushTo = float64(o.push) / chatsPushTurns
	o.pushGen++
	gen := o.pushGen
	time.AfterFunc(chatsPushIdle, func() { o.pushIdle(gen) })
	o.kickLocked()
}

func (o *orca) pushIdle(gen int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if gen != o.pushGen {
		return
	}
	o.push, o.pushTo = 0, 0
	o.kickLocked()
}

func (o *orca) kickLocked() {
	select {
	case o.kick <- struct{}{}:
	default:
	}
}

// tap is the tile tap and the click of крутилка справа, ряд 2: browse, or open the shown chat.
func (o *orca) tap() error {
	o.mu.Lock()
	browsing := o.browsing()
	o.mu.Unlock()
	if browsing {
		return o.open()
	}
	o.step(1)
	return nil
}

// hold is the tile hold: marks the shown chat read while browsing, else the freshest unread chat.
func (o *orca) hold() error {
	o.mu.Lock()
	var c orcaChat
	found := false
	if o.browsing() {
		c, found = o.shownLocked(), true
	} else {
		for _, sl := range orcaFlat(o.spaces) {
			if ch := o.spaces[sl.space].chats[sl.chat]; ch.unread {
				c, found = ch, true
				break
			}
		}
	}
	o.mu.Unlock()
	if !found {
		return nil
	}
	go func() {
		start := time.Now()
		if err := o.markRead(c.name); err != nil {
			log.Printf("orca: mark read %q: %v", c.name, err)
			return
		}
		log.Printf("orca: mark read %q in %s", c.name, time.Since(start).Round(time.Millisecond))
		o.refresh()
	}()
	return nil
}

// closeChat is the longer tile hold: closes the shown chat's tab in Orca, only while browsing.
func (o *orca) closeChat() error {
	o.mu.Lock()
	if !o.browsing() {
		o.mu.Unlock()
		return fmt.Errorf("orca: close needs a chat shown in the browse")
	}
	c := o.shownLocked()
	o.till = time.Now().Add(chatsBrowseFor)
	o.mu.Unlock()
	go func() {
		start := time.Now()
		if err := o.front(); err != nil {
			log.Printf("orca: close %q: %v", c.name, err)
			return
		}
		if err := o.closeTab(c.name); err != nil {
			log.Printf("orca: close %q: %v", c.name, err)
			return
		}
		log.Printf("orca: close %q in %s", c.name, time.Since(start).Round(time.Millisecond))
		o.refresh()
	}()
	return nil
}

// knob takes the press of крутилка справа, ряд 3 while browsing: the release opens the chat.
func (o *orca) knob(down bool) (bool, error) {
	o.mu.Lock()
	if down {
		o.knobDown = o.browsing()
		took := o.knobDown
		o.mu.Unlock()
		return took, nil
	}
	took := o.knobDown
	o.knobDown = false
	o.mu.Unlock()
	if !took {
		return false, nil
	}
	return true, o.open()
}

func (o *orca) open() error {
	o.mu.Lock()
	i := o.index()
	if !o.browsing() || i < 0 {
		o.mu.Unlock()
		return fmt.Errorf("orca: nothing picked")
	}
	s := o.spaces[i]
	c := s.chats[min(o.chat, len(s.chats)-1)]
	o.till = time.Now().Add(chatsBrowseFor)
	o.mu.Unlock()
	go func() {
		start := time.Now()
		if err := o.openChat(c); err != nil {
			log.Printf("orca: open %q / %q: %v", s.name, c.name, err)
			return
		}
		log.Printf("orca: open %q / %q in %s", s.name, c.name, time.Since(start).Round(time.Millisecond))
		o.refresh()
	}()
	return nil
}

func (o *orca) openChat(c orcaChat) error {
	if err := o.front(); err != nil {
		return err
	}
	return o.press(c.name)
}

func (o *orca) republish() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.publishLocked()
}

func (o *orca) publishLocked() {
	now := time.Now()
	if o.browsing() {
		c := o.shownLocked()
		o.vars.set("agents_svg", chatsPickImage([]chatEntry{c.entry()}, 0, 0, now))
		if key := orcaKey(c); key != o.shown {
			if !strings.HasPrefix(o.shown, c.id+"\x00") {
				o.scrollTo, o.scrollAt = 0, 0
				o.push, o.pushTo, o.pushAt = 0, 0, 0
				o.pushGen++
			}
			o.shown = key
			if _, ok := o.full[key]; !ok {
				go o.fetch(c)
			}
		}
		o.covered = true
		o.strip(orcaStrip(c, o.textLocked(c), o.scrollAt, o.pushAt))
		return
	}
	o.shown = ""
	if o.covered {
		o.covered = false
		o.strip("")
	}
	if len(o.spaces) == 0 {
		color := gray
		if o.failed {
			color = amber
		}
		o.vars.set("agents_svg", tileIcon("robot", color))
		return
	}
	flat := orcaFlat(o.spaces)
	n := min(len(flat), chatsOnTile)
	list := make([]chatEntry, n)
	for j, sl := range flat[:n] {
		list[j] = o.spaces[sl.space].chats[sl.chat].entry()
	}
	o.vars.set("agents_svg", chatsTileImage(list, len(flat) > chatsOnTile, now))
}

func orcaKey(c orcaChat) string {
	return c.id + "\x00" + c.text
}

func (o *orca) shownLocked() orcaChat {
	s := o.spaces[o.index()]
	return s.chats[min(o.chat, len(s.chats)-1)]
}

func (o *orca) textLocked(c orcaChat) string {
	return cmp.Or(orcaReply(o.full[orcaKey(c)]), orcaReply(c.text), "·")
}

// orcaReply is the strip text: the TLDR first without its label, two empty lines, then the rest of the reply; markdown formatted.
func orcaReply(s string) string {
	tldr, body := orcaTLDRSplit(s)
	tldr, body = orcaMarkdown(tldr), orcaMarkdown(body)
	if tldr == "" || body == "" {
		return tldr + body
	}
	return tldr + "\n\n\n" + body
}

var (
	orcaTLDR      = regexp.MustCompile(`(?i)^[*_#>\s]*tl;?dr\b`)
	orcaTLDRLabel = regexp.MustCompile(`(?i)^[*_#>\s]*tl;?dr[*_]*\s*[:：—–-]?[*_]*\s*`)
)

// orcaTLDRSplit cuts a trailing TLDR line (and whatever follows it) off the reply, its label removed; no TLDR leaves s as the body.
func orcaTLDRSplit(s string) (tldr, body string) {
	lines := strings.Split(strings.TrimRight(s, "\n "), "\n")
	for i := len(lines) - 1; i > 0; i-- {
		if orcaTLDR.MatchString(lines[i]) {
			lines[i] = orcaTLDRLabel.ReplaceAllString(lines[i], "")
			return strings.Join(lines[i:], "\n"), strings.Join(lines[:i], "\n")
		}
	}
	return "", s
}

type orcaRead struct {
	Result struct {
		Entries []struct {
			Kind string `json:"kind"`
			Role string `json:"role"`
			Text string `json:"text"`
		} `json:"entries"`
	} `json:"result"`
}

// orcaLastReply is the last assistant message of `orca agent read --json`; ps cuts it to 200 chars.
func orcaLastReply(data []byte) (string, error) {
	var r orcaRead
	if err := json.Unmarshal(data, &r); err != nil {
		return "", fmt.Errorf("agent read: %w", err)
	}
	for i := len(r.Result.Entries) - 1; i >= 0; i-- {
		if e := r.Result.Entries[i]; e.Kind == "message" && e.Role == "assistant" && strings.TrimSpace(e.Text) != "" {
			return e.Text, nil
		}
	}
	return "", nil
}

func (o *orca) fetch(c orcaChat) {
	out, err := o.run(append(append([]string{"agent", "read"}, c.env...), "orca_session_id:"+c.id, "--json")...)
	text := ""
	if err == nil {
		text, err = orcaLastReply(out)
	}
	if err != nil {
		log.Printf("orca: read %q: %v", c.name, err)
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.full) > 200 {
		clear(o.full)
	}
	o.full[orcaKey(c)] = text
	if o.browsing() && o.shown == orcaKey(c) {
		o.publishLocked()
	}
}

// scroll moves the shown reply a line per detent (крутилка справа, ряд 1), only while browsing.
func (o *orca) scroll(d int) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.browsing() {
		return false
	}
	o.till = time.Now().Add(chatsBrowseFor)
	end := o.scrollEndLocked()
	if d > 0 && o.scrollTo >= end || d < 0 && o.scrollTo <= 0 {
		o.pushLocked(d)
		return true
	}
	if o.push != 0 {
		o.push, o.pushTo = 0, 0
		o.pushGen++
	}
	o.scrollTo = min(max(o.scrollTo+float64(d)*orcaLineH, 0), end)
	o.kickLocked()
	return true
}

func (o *orca) animate() {
	for range o.kick {
		ticker := time.NewTicker(time.Second / animationHz)
		for o.scrollStep() {
			<-ticker.C
		}
		ticker.Stop()
	}
}

func (o *orca) scrollStep() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.browsing() {
		return false
	}
	moving := ease(&o.scrollAt, o.scrollTo, 0.5)
	moving = ease(&o.pushAt, o.pushTo, 0.02) || moving
	o.publishLocked()
	return moving
}

func orcaStrip(c orcaChat, text string, offset, bar float64) string {
	const w, h = stripDrawH, stripDrawW
	lines := orcaRichLines(text, w-8, orcaTextSz)
	uri, err := orcaStripPNG(w, h, bar,
		calText{text: chatsLine(c.name), x: 4, y: -offset, w: w - 8, h: orcaTitleH, size: 12, rgb: whiteRGB},
		calText{text: text, x: 4, y: orcaTitleH - offset, w: w - 8, h: max(float64(lines)*orcaLineH, h-orcaTitleH), size: orcaTextSz, rgb: grayRGB, rich: true})
	if err != nil {
		log.Print(err)
	}
	return uri
}
