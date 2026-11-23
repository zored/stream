package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	orcaCLI      = "/Applications/Orca.app/Contents/Resources/bin/orca"
	orcaPoll     = 15 * time.Second
	orcaTimeout  = 10 * time.Second
	orcaTabWait  = 6 * time.Second
	orcaTabEvery = 100 * time.Millisecond
)

type orcaChat struct {
	id, name, text string
	model          string
	unread         bool
	at             time.Time
}

type orcaSpace struct {
	id, name, kind string
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

// entry is the workspace card; the badge bottom-left is the icon of its chat number `chat` (the freshest outside browse).
func (s orcaSpace) entry(chat int) chatEntry {
	return chatEntry{app: "orca", id: s.id, title: s.name, unread: s.unread(), at: s.at(), sender: orcaIconPNG(s.chats[min(chat, len(s.chats)-1)])}
}

var orcaModels = map[string]string{"claude": "claude", "codex": "openai"}

type orcaPS struct {
	Result struct {
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
				Activity             struct {
					Listed bool   `json:"listed"`
					Unread bool   `json:"unread"`
					SortAt int64  `json:"sortAt"`
					Title  string `json:"title"`
				} `json:"activity"`
			} `json:"agents"`
		} `json:"worktrees"`
	} `json:"result"`
}

// orcaParse turns `orca worktree ps --json` into workspaces with chats, freshest chat activity first on both levels.
func orcaParse(data []byte) ([]orcaSpace, error) {
	var ps orcaPS
	if err := json.Unmarshal(data, &ps); err != nil {
		return nil, fmt.Errorf("worktree ps: %w", err)
	}
	var spaces []orcaSpace
	for _, w := range ps.Result.Worktrees {
		if w.IsArchived {
			continue
		}
		s := orcaSpace{id: w.WorktreeID, name: w.DisplayName, kind: w.WorkspaceKind, active: w.IsActive}
		for _, a := range w.Agents {
			if !a.Activity.Listed {
				continue
			}
			s.chats = append(s.chats, orcaChat{
				id:     a.OrcaSessionID,
				model:  orcaModels[a.AgentType],
				name:   cmp.Or(a.Activity.Title, a.ConversationName),
				text:   cmp.Or(a.LastAssistantMessage, a.Prompt),
				unread: a.Activity.Unread,
				at:     time.UnixMilli(a.Activity.SortAt),
			})
		}
		if len(s.chats) == 0 {
			continue
		}
		slices.SortStableFunc(s.chats, func(a, b orcaChat) int { return b.at.Compare(a.at) })
		spaces = append(spaces, s)
	}
	slices.SortStableFunc(spaces, func(a, b orcaSpace) int { return b.at().Compare(a.at()) })
	return spaces, nil
}

type orca struct {
	mu       sync.Mutex
	vars     *vars
	strip    func(uri string)
	run      func(args ...string) ([]byte, error)
	press    func(name string) error
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
	return &orca{vars: v, strip: strip, run: orcaRun, press: orcaPressTab, front: orcaFront, wake: make(chan struct{}, 1),
		full: map[string]string{}, kick: make(chan struct{}, 1)}
}

func orcaRun(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), orcaTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, orcaCLI, args...)
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
		out, err := o.run("worktree", "ps", "--json")
		var spaces []orcaSpace
		if err == nil {
			spaces, err = orcaParse(out)
		}
		o.mu.Lock()
		o.failed = err != nil
		if err != nil {
			if err.Error() != lastError {
				lastError = err.Error()
				log.Printf("orca: %v", err)
			}
		} else {
			lastError = ""
			o.spaces = spaces
			if o.browsing() && o.index() < 0 {
				o.closeLocked()
			}
		}
		o.publishLocked()
		o.mu.Unlock()
		select {
		case <-o.wake:
		case <-time.After(orcaPoll):
		}
	}
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

// chatStep walks the chats of the shown workspace (крутилка справа, ряд 3), only while browsing.
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

// moveChatLocked walks the chats through the workspaces in a loop: past the last chat comes the next workspace's first, before the first the previous workspace's last; toEnd lands at the end of the reply.
func (o *orca) moveChatLocked(d int, toEnd bool) {
	i := o.index()
	to := o.chat + d
	switch n := len(o.spaces[i].chats); {
	case to >= 0 && to < n:
		o.chat = to
	case len(o.spaces) == 1:
		o.chat = (to%n + n) % n
	default:
		next := o.spaces[agentStep(i, d, len(o.spaces))]
		o.space, o.chat = next.id, 0
		if d < 0 {
			o.chat = len(next.chats) - 1
		}
	}
	o.shown = ""
	o.publishLocked()
	if toEnd {
		o.scrollTo = o.scrollEndLocked()
		o.scrollAt = o.scrollTo
		o.publishLocked()
	}
}

func (o *orca) scrollEndLocked() float64 {
	lines := chatsTextLines(o.textLocked(o.shownLocked()), stripDrawH-8, orcaTextSz)
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
	o.closeLocked()
	o.publishLocked()
	o.mu.Unlock()
	if s.kind != "git" && !s.active {
		return fmt.Errorf("orca: %s workspace %q cannot be activated from the CLI", s.kind, s.name)
	}
	go func() {
		start := time.Now()
		if err := o.openChat(s, c); err != nil {
			log.Printf("orca: open %q / %q: %v", s.name, c.name, err)
			return
		}
		log.Printf("orca: open %q / %q in %s", s.name, c.name, time.Since(start).Round(time.Millisecond))
		o.refresh()
	}()
	return nil
}

func (o *orca) openChat(s orcaSpace, c orcaChat) error {
	if err := o.front(); err != nil {
		return err
	}
	if s.kind == "git" {
		if _, err := o.run("worktree", "activate", "--worktree", "id:"+s.id, "--json"); err != nil {
			return err
		}
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
		i := o.index()
		list := make([]chatEntry, len(o.spaces))
		for j, s := range o.spaces {
			list[j] = s.entry(0)
		}
		list[i] = o.spaces[i].entry(o.chat)
		o.vars.set("agents_svg", chatsPickImage(list, i, 0, now))
		c := o.shownLocked()
		if key := orcaKey(c); key != o.shown {
			o.shown, o.scrollTo, o.scrollAt = key, 0, 0
			o.push, o.pushTo, o.pushAt = 0, 0, 0
			o.pushGen++
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
	n := min(len(o.spaces), chatsOnTile)
	list := make([]chatEntry, n)
	for j, s := range o.spaces[:n] {
		list[j] = s.entry(0)
	}
	o.vars.set("agents_svg", chatsTileImage(list, len(o.spaces) > chatsOnTile, now))
}

func orcaKey(c orcaChat) string {
	return c.id + "\x00" + c.text
}

func (o *orca) shownLocked() orcaChat {
	s := o.spaces[o.index()]
	return s.chats[min(o.chat, len(s.chats)-1)]
}

func (o *orca) textLocked(c orcaChat) string {
	return cmp.Or(chatsLine(o.full[orcaKey(c)]), chatsLine(c.text), "·")
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
	out, err := o.run("agent", "read", "orca_session_id:"+c.id, "--json")
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

// scroll moves the shown reply half a line per detent (крутилка справа, ряд 1), only while browsing.
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
	o.scrollTo = min(max(o.scrollTo+float64(d)*orcaLineH/2, 0), end)
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
	lines := chatsTextLines(text, w-8, orcaTextSz)
	uri, err := orcaStripPNG(w, h, bar,
		calText{text: chatsLine(c.name), x: 4, y: -offset, w: w - 8, h: orcaTitleH, size: 12, rgb: whiteRGB},
		calText{text: text, x: 4, y: orcaTitleH - offset, w: w - 8, h: max(float64(lines)*orcaLineH, h-orcaTitleH), size: orcaTextSz, rgb: grayRGB})
	if err != nil {
		log.Print(err)
	}
	return uri
}
