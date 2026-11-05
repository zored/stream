package main

import (
	"encoding/json"
	"errors"
	"fmt"
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
	agentLedHz     = 20
	agentBreathe   = 2 * time.Second
	agentMaxDots   = 16
	agentDotR      = 8
	agentActiveR   = 9.5
	agentSlide     = 150 * time.Millisecond
	agentFrontFreq = time.Second
	agentKnobFor   = 5 * time.Second
	agentKnobRest  = 300 * time.Millisecond
	agentManual    = 2 * time.Second
	agentLedDone   = 0x404040
	agentLedIdle   = 0xFFFFFF
	agentLedRed    = 0xFF
	agentLedGreen  = 0xB0
	agentLedFloor  = 0.12
	agentCheckFreq = 10 * time.Second
)

var (
	desktopSession = regexp.MustCompile(`^local_[A-Za-z0-9-]{1,64}$`)
	itermSession   = regexp.MustCompile(`^w\d+t\d+p\d+:([A-F0-9-]+)$`)
	bundleID       = regexp.MustCompile(`^[A-Za-z0-9.-]+$`)
)

type agentSession struct {
	ID    string    `json:"id"`
	Kind  string    `json:"kind"`
	Since time.Time `json:"since"`
	Cwd   string    `json:"cwd"`
	Host  string    `json:"host,omitempty"`
	ITerm string    `json:"iterm,omitempty"`
	App   string    `json:"app,omitempty"`
	Pid   int       `json:"pid,omitempty"`
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
	mu       sync.Mutex
	sessions map[string]*agentSession
	cursor   *agentSession
	ttl      time.Duration
	vars     *vars
	breathe  chan struct{}
	slide    chan struct{}
	desktop  *desktopSessions
	ringFrom string
	ringTo   string
	ringAt   time.Time
	front    int
	polled   time.Time
	knobTill time.Time
	knobOpen *time.Timer
	manual   time.Time
}

func newAgents(v *vars, ttl time.Duration, desktop *desktopSessions) *agents {
	return &agents{sessions: map[string]*agentSession{}, ttl: ttl, vars: v, breathe: make(chan struct{}, 1), slide: make(chan struct{}, 1), desktop: desktop}
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
		if known && s.desktopDone() {
			drop = !rec.unread()
		} else {
			drop = time.Since(s.Since) > a.ttl || s.Pid > 0 && errors.Is(syscall.Kill(s.Pid, 0), syscall.ESRCH)
		}
		if drop {
			delete(a.sessions, id)
			changed = true
			continue
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

var agentRank = map[string]int{agentInput: 0, agentDone: 1, agentWorking: 2}

func agentLess(x, y *agentSession) int {
	if c := agentRank[x.Kind] - agentRank[y.Kind]; c != 0 {
		return c
	}
	if c := x.Since.Compare(y.Since); c != 0 {
		return c
	}
	return strings.Compare(x.ID, y.ID)
}

func (a *agents) sorted() []*agentSession {
	list := make([]*agentSession, 0, len(a.sessions))
	for _, s := range a.sessions {
		list = append(list, s)
	}
	slices.SortFunc(list, agentLess)
	return list
}

func (a *agents) focus() error {
	a.mu.Lock()
	list := a.sorted()
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
	a.knobTill, a.manual = time.Now().Add(agentKnobFor), time.Now()
	if next.Kind == agentDone && !next.desktopDone() {
		delete(a.sessions, next.ID)
	}
	target := *next
	a.mu.Unlock()
	log.Printf("agents: focus %s %s %s", target.ID, target.Kind, filepath.Base(target.Cwd))
	a.publish()
	return target.open()
}

func (a *agents) browsing() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return time.Now().Before(a.knobTill)
}

func (a *agents) step(d int) {
	a.mu.Lock()
	list := a.sorted()
	if len(list) == 0 {
		a.mu.Unlock()
		return
	}
	i := agentStep(slices.IndexFunc(list, func(s *agentSession) bool { return a.cursor != nil && s.ID == a.cursor.ID }), d, len(list))
	a.cursor = list[i]
	a.knobTill, a.manual = time.Now().Add(agentKnobFor), time.Now()
	target := *list[i]
	if a.knobOpen != nil {
		a.knobOpen.Stop()
	}
	a.knobOpen = time.AfterFunc(agentKnobRest, func() {
		log.Printf("agents: knob %s %s %s", target.ID, target.Kind, filepath.Base(target.Cwd))
		if err := target.open(); err != nil {
			log.Printf("agents: %v", err)
		}
	})
	a.mu.Unlock()
	a.publish()
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

func (a *agents) counts() (input, done int) {
	for _, s := range a.sessions {
		switch s.Kind {
		case agentInput:
			input++
		case agentDone:
			done++
		}
	}
	return
}

func (a *agents) publish() {
	a.mu.Lock()
	input, done := a.counts()
	list := a.sorted()
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
	kinds := make([]string, len(list))
	current, prev := -1, -1
	for i, s := range list {
		kinds[i] = s.Kind
		switch s.ID {
		case a.ringTo:
			current = i
		case a.ringFrom:
			prev = i
		}
	}
	t := a.slideT()
	a.mu.Unlock()
	a.vars.set("agents_svg", agentsSVG(kinds, current, prev, t))
	if input > 0 {
		select {
		case a.breathe <- struct{}{}:
		default:
		}
		return
	}
	a.vars.set("agents_led", strconv.Itoa(map[bool]int{true: agentLedDone, false: agentLedIdle}[done > 0]))
}

func (a *agents) animate() {
	for range a.breathe {
		start := time.Now()
		ticker := time.NewTicker(time.Second / agentLedHz)
		for range ticker.C {
			a.mu.Lock()
			input, _ := a.counts()
			a.mu.Unlock()
			if input == 0 {
				break
			}
			phase := float64(time.Since(start)) / float64(agentBreathe)
			level := agentLedFloor + (1-agentLedFloor)*(0.5-0.5*math.Cos(2*math.Pi*phase))
			r, g := int(agentLedRed*level), int(agentLedGreen*level)
			a.vars.set("agents_led", strconv.Itoa(r<<16|g<<8))
		}
		ticker.Stop()
		a.publish()
	}
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
	if time.Since(a.manual) < agentManual+agentKnobRest {
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
			delete(a.sessions, hit.ID)
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

func agentDot(i, n int) (x, y float64) {
	rows := (n + 3) / 4
	row, col := i/4, i%4
	inRow := min(4, n-row*4)
	return 50 + (float64(col)-float64(inRow-1)/2)*22, 50 + (float64(row)-float64(rows-1)/2)*22
}

func agentsSVG(kinds []string, current, prev int, t float64) string {
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">`)
	switch n := len(kinds); {
	case n == 0:
		fmt.Fprintf(&b, `<circle cx="50" cy="50" r="7" fill="none" stroke="%s" stroke-width="2.5"/>`, gray)
	case n > agentMaxDots:
		fill := "#fff"
		if kinds[0] == agentInput {
			fill = amber
		}
		fmt.Fprintf(&b, `<text x="50" y="50" dy="0.35em" text-anchor="middle" font-family="JetBrains Mono" font-size="56" fill="%s">%d</text>`, fill, n)
	default:
		e := t * t * (3 - 2*t)
		for i, kind := range kinds {
			x, y := agentDot(i, n)
			r := float64(agentDotR)
			if i == current {
				r += (agentActiveR - agentDotR) * e
			}
			if i == prev {
				r += (agentActiveR - agentDotR) * (1 - e)
			}
			fill := gray
			switch kind {
			case agentInput:
				fill = amber
			case agentDone:
				fill = "#fff"
			}
			fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s"/>`, x, y, r, fill)
		}
		if current >= 0 {
			x, y := agentDot(current, n)
			if prev >= 0 {
				px, py := agentDot(prev, n)
				x, y = px+(x-px)*e, py+(y-py)*e
			}
			fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="11.5" fill="none" stroke="#fff" stroke-width="1.5"/>`, x, y)
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
