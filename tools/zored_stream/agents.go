package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log"
	"math"
	"net/http"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	agentInput     = "input"
	agentDone      = "done"
	agentWorking   = "working"
	agentRead      = "read"
	agentMaxDots   = 16
	agentDotR      = 8
	agentActiveR   = 9.5
	agentSlide     = 150 * time.Millisecond
	agentFrontFreq = time.Second
	agentReadFor   = time.Minute
	agentManual    = 2 * time.Second
	agentStripFor  = 2 * time.Second
	agentCheckFreq = 10 * time.Second
)

var (
	desktopSession = regexp.MustCompile(`^local_[A-Za-z0-9-]{1,64}$`)
	itermSession   = regexp.MustCompile(`^w\d+t\d+p\d+:([A-F0-9-]+)$`)
	bundleID       = regexp.MustCompile(`^[A-Za-z0-9.-]+$`)
)

type agentSession struct {
	ID      string    `json:"id"`
	Kind    string    `json:"kind"`
	Since   time.Time `json:"since"`
	Cwd     string    `json:"cwd"`
	Host    string    `json:"host,omitempty"`
	ITerm   string    `json:"iterm,omitempty"`
	App     string    `json:"app,omitempty"`
	Pid     int       `json:"pid,omitempty"`
	Updated time.Time `json:"updated"`
	Child   bool      `json:"child,omitempty"`
	choicePayload
}

type choicePayload struct {
	Questions  []choiceQuestion `json:"questions,omitempty"`
	Permission string           `json:"permission,omitempty"`
	Step       int              `json:"step,omitempty"`
	Picked     []map[int]bool   `json:"-"`
}

type choiceQuestion struct {
	Question    string `json:"question"`
	MultiSelect bool   `json:"multiSelect"`
	Options     []struct {
		Label string `json:"label"`
	} `json:"options"`
}

type hookEvent struct {
	SessionID        string          `json:"session_id"`
	Event            string          `json:"hook_event_name"`
	Cwd              string          `json:"cwd"`
	ToolName         string          `json:"tool_name"`
	NotificationType string          `json:"notification_type"`
	ToolInput        json.RawMessage `json:"tool_input"`
}

type agents struct {
	mu                   sync.Mutex
	sessions             map[string]*agentSession
	cursor               *agentSession
	ttl                  time.Duration
	vars                 *vars
	slide                chan struct{}
	desktop              *desktopSessions
	ringFrom             string
	ringTo               string
	ringAt               time.Time
	front                int
	polled               time.Time
	manual               time.Time
	strip                func(uri string)
	stripOff             *time.Timer
	row                  []string
	rowWake              chan struct{}
	rowShown             bool
	rowOffset, rowTarget float64
	rowTurn              float64
	rowMirror            bool
	rowTitle             string
	rowPos               []float64
	overlay              *stripOverlay
	contexts             *contexts
	onContext            func(*chatContext)
	contextErr           string
}

func newAgents(v *vars, ttl time.Duration, desktop *desktopSessions) *agents {
	return &agents{sessions: map[string]*agentSession{}, ttl: ttl, vars: v, slide: make(chan struct{}, 1), rowWake: make(chan struct{}, 1), desktop: desktop}
}

func (s *agentSession) desktopDone() bool {
	return s.Kind == agentDone && desktopSession.MatchString(s.Host)
}

func (a *agents) serveEvent(w http.ResponseWriter, r *http.Request) {
	var e hookEvent
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		http.Error(w, "hook json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if e.SessionID == "" || e.Event == "" {
		http.Error(w, "session_id and hook_event_name required", http.StatusBadRequest)
		return
	}
	pid, _ := strconv.Atoi(r.Header.Get("Agent-Pid"))
	s := agentSession{ID: e.SessionID, Cwd: e.Cwd, Host: r.Header.Get("Agent-Host"), ITerm: r.Header.Get("Agent-Iterm"),
		App: r.Header.Get("Agent-App"), Pid: pid}
	switch e.Event {
	case "PermissionRequest", "PreToolUse":
		if e.ToolName != "AskUserQuestion" {
			if e.Event == "PermissionRequest" {
				s.Permission = e.ToolName
				a.mark(s, agentInput)
			}
			break
		}
		var in struct {
			Questions []choiceQuestion `json:"questions"`
		}
		if err := json.Unmarshal(e.ToolInput, &in); err != nil {
			http.Error(w, "AskUserQuestion tool_input: "+err.Error(), http.StatusBadRequest)
			return
		}
		s.Questions = in.Questions
		a.mark(s, agentInput)
	case "Notification":
		switch e.NotificationType {
		case "permission_prompt", "elicitation_dialog":
			a.mark(s, agentInput)
		case "idle_prompt":
			a.mark(s, agentDone)
		}
	case "Stop", "StopFailure":
		a.mark(s, agentDone)
	case "PostToolUse", "UserPromptSubmit":
		a.mark(s, agentWorking)
	case "SessionStart", "SessionEnd":
		a.remove(e.SessionID)
	default:
		log.Printf("agents: unexpected hook %s", e.Event)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *agents) mark(s agentSession, kind string) {
	a.mu.Lock()
	old := a.sessions[s.ID]
	if old != nil && old.Kind == kind {
		if old.Questions == nil && old.Permission == "" {
			old.choicePayload = s.choicePayload
		}
		a.mu.Unlock()
		return
	}
	s.Kind, s.Since = kind, time.Now()
	s.Updated = s.Since
	a.enrich(&s)
	s.Picked = make([]map[int]bool, len(s.Questions))
	for i := range s.Picked {
		s.Picked[i] = map[int]bool{}
	}
	for id, o := range a.sessions {
		if s.Host != "" && o.Host == s.Host && id != s.ID {
			delete(a.sessions, id)
		}
	}
	a.sessions[s.ID] = &s
	a.mu.Unlock()
	log.Printf("agents: %s %s %s", s.ID, kind, filepath.Base(s.Cwd))
	if kind != agentWorking {
		claude.wake()
	}
	a.publish()
}

func (a *agents) remove(id string) {
	a.mu.Lock()
	s, ok := a.sessions[id]
	ok = ok && !s.desktopDone()
	if ok {
		delete(a.sessions, id)
	}
	a.mu.Unlock()
	if ok {
		log.Printf("agents: %s cleared", id)
		a.publish()
	}
}

func (a *agents) expire() {
	recs, err := a.desktop.scan()
	if err != nil {
		log.Printf("agents: desktop sessions: %v", err)
	}
	a.mu.Lock()
	changed := false
	hosts := map[string]bool{}
	for id, s := range a.sessions {
		rec, known := recs[s.Host]
		var drop bool
		switch {
		case s.Kind == agentRead:
			drop = time.Since(s.Since) > agentReadFor || known && rec.unread()
		case known && s.desktopDone():
			if !rec.unread() {
				s.markRead()
				changed = true
			}
		default:
			drop = time.Since(s.Since) > a.ttl || s.Pid > 0 && errors.Is(syscall.Kill(s.Pid, 0), syscall.ESRCH)
		}
		if drop {
			delete(a.sessions, id)
			changed = true
			continue
		}
		if rec, ok := recs[s.Host]; ok {
			s.Updated, s.Child = time.UnixMilli(max(rec.LastActivityAt, rec.LastTurnEdgeAt)), rec.child()
		}
		hosts[s.Host] = true
	}
	for host, rec := range recs {
		if rec.unread() && !hosts[host] {
			s := rec.session()
			a.sessions[s.ID] = s
			changed = true
			log.Printf("agents: %s unread %s", s.ID, filepath.Base(s.Cwd))
		}
	}
	a.mu.Unlock()
	if changed {
		a.publish()
	}
}

func (s *agentSession) markRead() {
	s.Kind, s.Since = agentRead, time.Now()
}

func (a *agents) enrich(s *agentSession) {
	if rec, ok := a.desktop.record(s.Host); ok {
		s.Updated, s.Child = time.Now(), rec.child()
	}
}

func agentRank(kind string) int {
	switch kind {
	case agentInput:
		return 0
	case agentDone:
		return 1
	case agentWorking:
		return 2
	}
	return 3
}

func agentLess(x, y *agentSession) int {
	if c := agentRank(x.Kind) - agentRank(y.Kind); c != 0 {
		return c
	}
	if c := x.Updated.Compare(y.Updated); c != 0 {
		return c
	}
	return strings.Compare(x.ID, y.ID)
}

func (a *agents) ranked() []*agentSession {
	list := make([]*agentSession, 0, len(a.sessions))
	for _, s := range a.sessions {
		list = append(list, s)
	}
	slices.SortFunc(list, agentLess)
	return list
}

type agentNode struct {
	s     *agentSession
	root  string
	depth int
}

func (a *agents) sorted() []*agentSession {
	list, _ := a.grouped()
	return list
}

func (a *agents) grouped() ([]*agentSession, []int) {
	nodes := make([]agentNode, 0, len(a.sessions))
	for _, s := range a.sessions {
		root, depth := s.ID, 0
		if s.Host != "" {
			root = s.Host
			for depth < 16 {
				rec, ok := a.desktop.record(root)
				if !ok || rec.parent() == "" {
					break
				}
				root, depth = rec.parent(), depth+1
			}
		}
		nodes = append(nodes, agentNode{s, root, depth})
	}
	return agentGroups(nodes)
}

func agentGroups(nodes []agentNode) ([]*agentSession, []int) {
	fresh := map[string]time.Time{}
	for _, n := range nodes {
		if n.s.Updated.After(fresh[n.root]) {
			fresh[n.root] = n.s.Updated
		}
	}
	slices.SortFunc(nodes, func(x, y agentNode) int {
		if x.root != y.root {
			if c := fresh[y.root].Compare(fresh[x.root]); c != 0 {
				return c
			}
			return strings.Compare(x.root, y.root)
		}
		if c := y.depth - x.depth; c != 0 {
			return c
		}
		if c := y.s.Updated.Compare(x.s.Updated); c != 0 {
			return c
		}
		return strings.Compare(x.s.ID, y.s.ID)
	})
	list := make([]*agentSession, len(nodes))
	var sizes []int
	for i, n := range nodes {
		list[i] = n.s
		if i == 0 || n.root != nodes[i-1].root {
			sizes = append(sizes, 0)
		}
		sizes[len(sizes)-1]++
	}
	return list, sizes
}

func (a *agents) focus() error {
	a.mu.Lock()
	list := a.ranked()
	if len(list) == 0 {
		a.cursor = nil
		a.mu.Unlock()
		return newChat()
	}
	next := list[0]
	if a.cursor != nil {
		if i := slices.IndexFunc(list, func(s *agentSession) bool { return agentLess(s, a.cursor) > 0 }); i >= 0 {
			next = list[i]
		}
	}
	a.cursor = next
	a.manual = time.Now()
	if next.Kind == agentDone && !next.desktopDone() {
		next.markRead()
	}
	target := *next
	a.mu.Unlock()
	log.Printf("agents: focus %s %s %s", target.ID, target.Kind, filepath.Base(target.Cwd))
	a.publish()
	return target.open()
}

func (a *agents) step(d int) {
	a.addFront()
	a.mu.Lock()
	list := a.sorted()
	if len(list) == 0 {
		a.mu.Unlock()
		return
	}
	i := agentStep(slices.IndexFunc(list, func(s *agentSession) bool { return a.cursor != nil && s.ID == a.cursor.ID }), d, len(list))
	a.cursor = list[i]
	a.manual = time.Now()
	target := *list[i]
	a.showStrip(target)
	go func() {
		log.Printf("agents: knob %s %s %s", target.ID, target.Kind, filepath.Base(target.Cwd))
		if err := target.open(); err != nil {
			log.Printf("agents: %v", err)
		}
	}()
	a.mu.Unlock()
	a.publish()
}

func (a *agents) showStrip(s agentSession) {
	if a.strip == nil {
		return
	}
	rec, _ := a.desktop.record(s.Host)
	a.rowTitle = agentStripImage(rec.Title, filepath.Base(s.Cwd))
	a.strip(a.rowTitle)
	list, sizes := a.grouped()
	a.row = make([]string, len(list))
	a.rowPos = agentRowPos(sizes)
	for i, l := range list {
		r, _ := a.desktop.record(l.Host)
		a.row[i] = cmp.Or(titleEmoji(r.Title), agentRowDot[l.Kind], "⚪")
		if l.ID == s.ID {
			a.rowTarget = a.rowPos[i]
		}
	}
	if !a.rowShown {
		o, err := readOrientation()
		if err != nil {
			log.Printf("agents: %v", err)
		}
		a.rowTurn, a.rowMirror = upright(o, false), stripFlipped(o, false)
		a.rowOffset = a.rowTarget
	}
	a.rowShown = true
	select {
	case a.rowWake <- struct{}{}:
	default:
	}
	if a.stripOff != nil {
		a.stripOff.Stop()
	}
	a.stripOff = time.AfterFunc(agentStripFor, func() {
		a.mu.Lock()
		a.rowShown = false
		a.mu.Unlock()
		a.overlay.show("agents", false)
		a.strip("")
	})
}

func agentStripImage(title, cwd string) string {
	const w, h = stripDrawH, stripDrawW
	if title == "" {
		return calImage(w, h, true, calText{text: cwd, x: 4, y: 3, w: w - 8, h: 44, size: 15, rgb: whiteRGB})
	}
	return calImage(w, h, true, calText{text: title, tail: " " + cwd, x: 4, y: 3, w: w - 8, h: 44, size: 15, rgb: whiteRGB, tailRGB: grayRGB})
}

const agentGroupGap = 18

func agentRowPos(sizes []int) []float64 {
	var pos []float64
	y := 0.0
	for g, k := range sizes {
		if g > 0 {
			y += agentGroupGap
		}
		for range k {
			pos = append(pos, y)
			y += emojiPitch
		}
	}
	return pos
}

func agentColumn(row []string, pos []float64, offset, turn float64, mirror bool) []emojiGlyph {
	var out []emojiGlyph
	for i, e := range row {
		y := emojiTop + pos[i] - offset
		if y < -emojiPitch || y > stripDrawH+emojiPitch {
			continue
		}
		if mirror {
			y = stripDrawH - y
		}
		out = append(out, emojiGlyph{text: e, y: y, size: emojiSize, alpha: 1, turn: turn})
	}
	return out
}

var agentRowDot = map[string]string{agentInput: "🟡", agentDone: "⚪"}

func (a *agents) animateRow() {
	for range a.rowWake {
		ticker := time.NewTicker(time.Second / animationHz)
		for {
			a.mu.Lock()
			if !a.rowShown {
				a.mu.Unlock()
				break
			}
			d := a.rowTarget - a.rowOffset
			if math.Abs(d) < 0.5 {
				a.rowOffset = a.rowTarget
			} else {
				a.rowOffset += d * 0.3
			}
			row, pos, offset, done := a.row, a.rowPos, a.rowOffset, a.rowOffset == a.rowTarget
			turn, mirror, title := a.rowTurn, a.rowMirror, a.rowTitle
			a.mu.Unlock()
			uri, err := emojiPNG(agentColumn(row, pos, offset, turn, mirror), emojiRingY(mirror))
			if err != nil {
				log.Print(err)
				break
			}
			a.vars.set("emoji_l", uri)
			a.vars.set("emoji_r", title)
			a.overlay.show("agents", true)
			if done {
				break
			}
			<-ticker.C
		}
		ticker.Stop()
	}
}

func (a *agents) addFront() {
	if pid := frontPid(); pid == 0 || pid != claudePid() {
		return
	}
	rec, ok := a.desktop.front()
	if !ok {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, s := range a.sessions {
		if s.Host == rec.SessionID {
			return
		}
	}
	s := rec.session()
	s.markRead()
	a.sessions[s.ID] = s
	a.cursor = s
	log.Printf("agents: front %s added hollow %s", s.ID, filepath.Base(s.Cwd))
}

func agentStep(i, d, n int) int {
	if i < 0 {
		if d > 0 {
			return 0
		}
		return n - 1
	}
	return ((i+d)%n + n) % n
}

func (s agentSession) open() error {
	var cmd *exec.Cmd
	switch {
	case desktopSession.MatchString(s.Host):
		cmd = exec.Command("/usr/bin/open", "claude://code/continue?session="+s.Host)
	case itermSession.MatchString(s.ITerm):
		cmd = exec.Command("/usr/bin/osascript", "-e", fmt.Sprintf(itermFocus, itermSession.FindStringSubmatch(s.ITerm)[1]))
	case bundleID.MatchString(s.App):
		cmd = exec.Command("/usr/bin/open", "-b", s.App)
	default:
		return fmt.Errorf("agent %s: nothing to focus", s.ID)
	}
	if err := run(cmd); err != nil {
		return fmt.Errorf("focus %s: %w", s.ID, err)
	}
	return nil
}

func newChat() error {
	return run(exec.Command("/usr/bin/open", "claude://code/new"))
}

func run(cmd *exec.Cmd) error {
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %v: %s", cmd.Args, err, out)
	}
	return nil
}

const itermFocus = `tell application id "com.googlecode.iterm2"
	repeat with w in windows
		repeat with t in tabs of w
			repeat with s in sessions of t
				if unique id of s is "%s" then
					select w
					tell t to select
					tell s to select
					activate
					return
				end if
			end repeat
		end repeat
	end repeat
	error "iTerm2 session not found"
end tell`

func (a *agents) publish() {
	a.mu.Lock()
	list, sizes := a.grouped()
	cursor := ""
	if a.cursor != nil {
		cursor = a.cursor.ID
	}
	if cursor != a.ringTo {
		a.ringFrom, a.ringTo, a.ringAt = a.ringTo, cursor, time.Now()
		select {
		case a.slide <- struct{}{}:
		default:
		}
	}
	dots := make([]agentDotSpec, len(list))
	current, prev := -1, -1
	for i, s := range list {
		rec, _ := a.desktop.record(s.Host)
		dots[i] = agentDotSpec{s.Kind, titleEmoji(rec.Title)}
		switch s.ID {
		case a.ringTo:
			current = i
		case a.ringFrom:
			prev = i
		}
	}
	t := a.slideT()
	ctx := a.cursorContext()
	a.mu.Unlock()
	if a.onContext != nil {
		a.onContext(ctx)
	}
	a.vars.set("agents_svg", agentsSVG(dots, sizes, current, prev, t, ctx))
}

func (a *agents) cursorContext() *chatContext {
	if a.cursor == nil || a.contexts == nil {
		return nil
	}
	ctx, err := a.contexts.of(a.cursor.ID)
	if err != nil {
		if msg := err.Error(); msg != a.contextErr {
			a.contextErr = msg
			log.Printf("agents: context: %v", err)
		}
		return nil
	}
	a.contextErr = ""
	return &ctx
}

func (a *agents) slideT() float64 {
	return min(1, float64(time.Since(a.ringAt))/float64(agentSlide))
}

func (a *agents) animateSlide() {
	for range a.slide {
		ticker := time.NewTicker(time.Second / animationHz)
		for range ticker.C {
			a.publish()
			a.mu.Lock()
			done := a.slideT() >= 1
			a.mu.Unlock()
			if done {
				break
			}
		}
		ticker.Stop()
	}
}

func (a *agents) watchFront() {
	front := int(frontPid())
	changed := front != a.front
	a.front = front
	var match func(*agentSession) bool
	switch {
	case front != 0 && front == int(claudePid()):
		if !claude.due(&a.polled) {
			return
		}
		recs, err := a.desktop.scanIfChanged()
		if err != nil {
			log.Printf("agents: desktop sessions: %v", err)
			return
		}
		host := frontDesktop(recs)
		if host == "" {
			return
		}
		match = func(s *agentSession) bool { return s.Host == host }
	case changed && front != 0 && front == int(itermPid()):
		uid, err := itermCurrent()
		if err != nil {
			log.Printf("agents: %v", err)
			return
		}
		match = func(s *agentSession) bool { return itermUID(s.ITerm) == uid }
	default:
		return
	}
	a.mu.Lock()
	if time.Since(a.manual) < agentManual {
		a.mu.Unlock()
		return
	}
	var hit *agentSession
	for _, s := range a.sessions {
		if match(s) {
			hit = s
		}
	}
	same := hit == a.cursor || hit != nil && a.cursor != nil && hit.ID == a.cursor.ID
	if !same {
		a.cursor = hit
		if hit != nil && hit.Kind == agentDone && !hit.desktopDone() {
			hit.markRead()
		}
	}
	a.mu.Unlock()
	if !same {
		if hit != nil {
			log.Printf("agents: front %s %s %s", hit.ID, hit.Kind, filepath.Base(hit.Cwd))
		}
		a.publish()
	}
}

func frontDesktop(recs map[string]desktopRecord) string {
	var best desktopRecord
	for _, r := range recs {
		if !r.IsArchived && r.LastFocusedAt > best.LastFocusedAt {
			best = r
		}
	}
	return best.SessionID
}

func itermUID(s string) string {
	if m := itermSession.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

type agentPos struct{ x, y, r float64 }

func agentLayout(sizes []int) []agentPos {
	m := len(sizes)
	cols := int(math.Ceil(math.Sqrt(float64(m))))
	rows := (m + cols - 1) / cols
	cell := 88 / float64(max(cols, rows))
	var out []agentPos
	for g, k := range sizes {
		row, col := g/cols, g%cols
		inRow := min(cols, m-row*cols)
		cx := 50 + (float64(col)-float64(inRow-1)/2)*cell
		cy := 50 + (float64(row)-float64(rows-1)/2)*cell
		if k == 1 {
			out = append(out, agentPos{cx, cy, cell * 0.36})
			continue
		}
		ring := cell * 0.27
		r := min(cell*0.2, math.Pi*ring/float64(k)*0.9)
		for i := range k {
			a := -math.Pi/2 + 2*math.Pi*float64(i)/float64(k)
			out = append(out, agentPos{cx + ring*math.Cos(a), cy + ring*math.Sin(a), r})
		}
	}
	return out
}

type agentDotSpec struct {
	kind  string
	emoji string
}

var emojiSet = sync.OnceValue(func() map[string]string {
	set := map[string]string{}
	for _, g := range emojiGroups {
		for _, e := range g {
			set[e] = e
			set[strings.ReplaceAll(e, "\uFE0F", "")] = e
		}
	}
	return set
})

func titleEmoji(title string) string {
	r := []rune(strings.TrimSpace(title))
	for n := min(len(r), 10); n > 0; n-- {
		if e, ok := emojiSet()[string(r[:n])]; ok {
			return e
		}
	}
	return ""
}

func agentEmoji(b *strings.Builder, x, y, r float64, dot agentDotSpec) {
	opacity := 1.0
	switch dot.kind {
	case agentInput:
		agentShape(b, x, y, r, `fill="`+amber+`"`)
	case agentDone:
		agentShape(b, x, y, r, `fill="#fff"`)
	case agentWorking:
		agentShape(b, x, y, r, `fill="`+gray+`"`)
	case agentRead:
		agentShape(b, x, y, r-1, `fill="none" stroke="`+gray+`" stroke-width="2"`)
		opacity = 0.4
	}
	fmt.Fprintf(b, `<text x="%.1f" y="%.1f" dy="0.35em" text-anchor="middle" font-family="Apple Color Emoji" font-size="%.1f" opacity="%g">%s</text>`,
		x, y+r*0.6, r*2.1, opacity, html.EscapeString(dot.emoji))
}

func agentShape(b *strings.Builder, x, y, r float64, paint string) {
	fmt.Fprintf(b, `<circle cx="%.1f" cy="%.1f" r="%.1f" %s/>`, x, y, r, paint)
}

func agentsSVG(dots []agentDotSpec, sizes []int, current, prev int, t float64, ctx *chatContext) string {
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">`)
	if ctx != nil {
		fmt.Fprintf(&b, `<rect width="100" height="%d" fill="%s"/><rect width="%.1f" height="%d" fill="%s"/>`,
			contextBarH, stripTrack, min(ctx.Percent, 100), contextBarH, levelColors[levelOf(ctx.Percent)])
	}
	switch n := len(dots); {
	case n == 0:
		fmt.Fprintf(&b, `<circle cx="50" cy="50" r="7" fill="none" stroke="%s" stroke-width="2.5"/>`, gray)
	case n > agentMaxDots:
		fill := "#fff"
		for _, d := range dots {
			if d.kind == agentInput {
				fill = amber
			}
		}
		fmt.Fprintf(&b, `<text x="50" y="50" dy="0.35em" text-anchor="middle" font-family="JetBrains Mono" font-size="56" fill="%s">%d</text>`, fill, n)
	default:
		pos := agentLayout(sizes)
		for i, dot := range dots {
			p := pos[i]
			if dot.emoji != "" {
				agentEmoji(&b, p.x, p.y, p.r, dot)
				continue
			}
			switch dot.kind {
			case agentInput:
				agentShape(&b, p.x, p.y, p.r, `fill="`+amber+`"`)
			case agentDone:
				agentShape(&b, p.x, p.y, p.r, `fill="#fff"`)
			case agentRead:
				agentShape(&b, p.x, p.y, p.r-1, `fill="none" stroke="`+gray+`" stroke-width="2"`)
			default:
				agentShape(&b, p.x, p.y, p.r, `fill="`+gray+`"`)
			}
		}
		if current >= 0 {
			e := t * t * (3 - 2*t)
			p := pos[current]
			if prev >= 0 {
				q := pos[prev]
				p = agentPos{q.x + (p.x-q.x)*e, q.y + (p.y-q.y)*e, q.r + (p.r-q.r)*e}
			}
			fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="%.1f" fill="none" stroke="#fff" stroke-width="1.5"/>`, p.x, p.y, p.r+2.5)
		}
	}
	b.WriteString(`</svg>`)
	return dataURI(b.String())
}

func (a *agents) serveJSON(w http.ResponseWriter, _ *http.Request) {
	a.mu.Lock()
	list := a.sorted()
	out := make([]agentSession, len(list))
	for i, s := range list {
		out[i] = *s
		out[i].Cwd = filepath.Base(s.Cwd)
	}
	a.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		log.Printf("agents json: %v", err)
	}
}

func (a *agents) hasCLIChoice() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, s := range a.sessions {
		if s.Kind == agentInput && s.ITerm != "" && (s.Questions != nil || s.Permission != "") {
			return true
		}
	}
	return false
}

func (a *agents) cliCard(uid string) (*choiceCard, string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, s := range a.sessions {
		m := itermSession.FindStringSubmatch(s.ITerm)
		if s.Kind != agentInput || m == nil || m[1] != uid {
			continue
		}
		if s.Questions == nil {
			if s.Permission == "" {
				return nil, ""
			}
			return &choiceCard{Options: []choiceOption{{Label: "yes"}}, Index: 1, Count: 1}, s.ID
		}
		step := min(max(s.Step, 0), len(s.Questions)-1)
		q := s.Questions[step]
		card := &choiceCard{Multi: q.MultiSelect, Index: step + 1, Count: len(s.Questions)}
		for i, o := range q.Options {
			card.Options = append(card.Options, choiceOption{Label: o.Label, Selected: s.Picked[step][i]})
		}
		return card, s.ID
	}
	return nil, ""
}

func (a *agents) cliStep(id string, d int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if s := a.sessions[id]; s != nil {
		s.Step = min(max(s.Step+d, 0), len(s.Questions)-1)
	}
}

func (a *agents) cliToggle(id string, i int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if s := a.sessions[id]; s != nil && s.Step < len(s.Picked) {
		s.Picked[s.Step][i] = !s.Picked[s.Step][i]
	}
}
