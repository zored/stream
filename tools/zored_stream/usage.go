package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	usageURL           = "https://api.anthropic.com/api/oauth/usage"
	usageKeychain      = "Claude Code-credentials"
	usageMinRefresh    = time.Minute
	usageMaxBackoff    = time.Hour
	usageTick          = time.Minute
	usageKeychainRetry = time.Minute
	usageOfflineRetry  = time.Minute
	usageMinRetry      = time.Minute
	workdays           = 5
	day                = 24 * time.Hour
	stripHz            = 60
	stripEasing        = 0.293
	stripDrawW         = 48
	stripDrawH         = 258
	stripSessionW      = 32
	tilePx             = 90
	stripAllowed       = "#48484A"
	stripFable         = "#BF5AF2"
	stripClockFont     = 12.8
	stripDaysFont      = 10
)

var (
	aiStateColors = map[string]string{"ok": "#fff", "pending": gray, "preview": gray, "idle": "#fff", "other": amber, "error": red}
	aiEffortDots  = []string{"low", "medium", "high", "extra", "max"}
	levelColors   = map[string]string{"low": "#fff", "normal": "#fff", "high": "#FFD94D", "critical": "#FF6666", "over": "#B3261E"}
)

type limit struct {
	Kind     string      `json:"kind"`
	Percent  float64     `json:"percent"`
	ResetsAt *time.Time  `json:"resets_at"`
	Scope    *limitScope `json:"scope"`
}

type limitScope struct {
	Model *limitModel `json:"model"`
}

type limitModel struct {
	DisplayName string `json:"display_name"`
}

func (l limit) model() string {
	if l.Scope == nil || l.Scope.Model == nil {
		return ""
	}
	return l.Scope.Model.DisplayName
}

type usageSnapshot struct {
	Limits    []json.RawMessage `json:"limits"`
	FetchedAt *time.Time        `json:"fetched_at"`
	Error     string            `json:"error,omitempty"`
	NextAt    time.Time         `json:"next_at"`
}

type usage struct {
	mu           sync.Mutex
	every        time.Duration
	http         *http.Client
	vars         *vars
	snap         usageSnapshot
	limits       []limit
	failures     int
	forceRefresh bool
	wake         chan struct{}
	label        stripLabel
	note         stripNote
	noteTill     time.Time
	covered      string
	frame        stripFrame
	context      *chatContext
}

type stripFrame struct {
	label   stripLabel
	note    stripNote
	failed  bool
	stale   bool
	session float64
	week    float64
	fable   float64
	allowed float64
	weekLvl string
	left5h  string
	leftW   string
}

type stripNote struct{ text, color string }

type stripLabel struct{ model, effort, state string }

func newUsage(v *vars, every time.Duration, proxy string) (*usage, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxy != "" {
		u, err := url.Parse(proxy)
		if err != nil {
			return nil, fmt.Errorf("usage proxy: %w", err)
		}
		transport.Proxy = http.ProxyURL(u)
	}
	return &usage{every: every, vars: v, wake: make(chan struct{}, 1),
		http: &http.Client{Timeout: 20 * time.Second, Transport: transport}}, nil
}

func (u *usage) run() {
	for {
		u.fetch()
		u.sleepUntilDue()
	}
}

func (u *usage) sleepUntilDue() {
	for {
		u.mu.Lock()
		wait := usageWait(u.snap.NextAt, time.Now())
		u.mu.Unlock()
		if wait <= 0 {
			return
		}
		select {
		case <-time.After(wait):
		case <-u.wake:
			return
		}
	}
}

// Go timers stop while the Mac sleeps, so wait in short ticks against the wall clock.
func usageWait(next, now time.Time) time.Duration {
	return min(next.Round(0).Sub(now.Round(0)), usageTick)
}

func (u *usage) refresh() error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.snap.FetchedAt != nil && time.Since(*u.snap.FetchedAt) < usageMinRefresh {
		return nil
	}
	select {
	case u.wake <- struct{}{}:
	default:
	}
	return nil
}

func (u *usage) focus(label stripLabel) {
	u.mu.Lock()
	changed := u.label != label
	u.label = label
	u.mu.Unlock()
	if changed {
		u.publish()
	}
}

func (u *usage) say(note stripNote, d time.Duration) {
	u.mu.Lock()
	u.note, u.noteTill = note, time.Now().Add(d)
	u.mu.Unlock()
	time.AfterFunc(d, u.publish)
	u.publish()
}

func (u *usage) cover(uri string) {
	u.mu.Lock()
	u.covered = uri
	u.mu.Unlock()
	u.draw()
}

func (u *usage) draw() {
	u.mu.Lock()
	f, covered := u.frame, u.covered
	u.mu.Unlock()
	if covered != "" {
		u.vars.set("ai_strip_svg", covered)
		return
	}
	u.vars.set("ai_strip_svg", aiStripSVG(f))
}

func ease(shown *float64, target, snap float64) bool {
	*shown += (target - *shown) * stripEasing
	if math.Abs(target-*shown) < snap {
		*shown = target
	}
	return *shown != target
}

func (u *usage) fetch() {
	raw, limits, retryAfter, err := u.get()
	now := time.Now()
	u.mu.Lock()
	if err != nil {
		u.snap.Error = err.Error()
		next, counted := usageNextTry(err, u.failures+1, retryAfter, now)
		if counted {
			u.failures++
		}
		u.snap.NextAt = next
		log.Printf("usage: %v, next try at %s", err, u.snap.NextAt.Format(time.TimeOnly))
	} else {
		u.failures = 0
		u.snap = usageSnapshot{Limits: raw, FetchedAt: &now, NextAt: now.Add(u.every)}
		u.limits = limits
	}
	u.mu.Unlock()
	u.publish()
}

// Local failures (keychain, no route to the proxy) and timeouts retry every minute; server errors back off from a minute.
func usageNextTry(err error, failures int, retryAfter time.Duration, now time.Time) (time.Time, bool) {
	switch {
	case errors.Is(err, errKeychain):
		return now.Add(usageKeychainRetry), false
	case offline(err):
		return now.Add(usageOfflineRetry), false
	}
	return now.Add(max(min(usageMinRetry<<(failures-1), usageMaxBackoff), retryAfter)), true
}

func offline(err error) bool {
	var op *net.OpError
	var ne net.Error
	return errors.As(err, &op) && (op.Op == "dial" || op.Op == "proxyconnect") || errors.As(err, &ne) && ne.Timeout()
}

func (u *usage) get() ([]json.RawMessage, []limit, time.Duration, error) {
	u.mu.Lock()
	force := u.forceRefresh
	u.mu.Unlock()
	token, err := u.claudeToken(force)
	if err != nil {
		return nil, nil, 0, err
	}
	req, err := http.NewRequest(http.MethodGet, usageURL, nil)
	if err != nil {
		return nil, nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	resp, err := u.http.Do(req)
	if err != nil {
		return nil, nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, 0, err
	}
	u.mu.Lock()
	u.forceRefresh = resp.StatusCode == http.StatusUnauthorized
	u.mu.Unlock()
	if resp.StatusCode != http.StatusOK {
		retry, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
		return nil, nil, time.Duration(retry) * time.Second, fmt.Errorf("%s: %.120s", resp.Status, body)
	}
	var data struct {
		Limits []json.RawMessage `json:"limits"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, nil, 0, err
	}
	if len(data.Limits) == 0 {
		return nil, nil, 0, fmt.Errorf("no limits in %.120s", body)
	}
	limits := make([]limit, len(data.Limits))
	for i, r := range data.Limits {
		if err := json.Unmarshal(r, &limits[i]); err != nil {
			return nil, nil, 0, fmt.Errorf("limit %s: %w", r, err)
		}
	}
	return data.Limits, limits, 0, nil
}

func (u *usage) serveJSON(w http.ResponseWriter, _ *http.Request) {
	u.mu.Lock()
	body, err := json.Marshal(struct {
		usageSnapshot
		Context *chatContext `json:"context"`
	}{u.snap, u.context})
	u.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

func (u *usage) setContext(ctx *chatContext) {
	u.mu.Lock()
	u.context = ctx
	u.mu.Unlock()
}

func limitsStale(failed bool, fetched *time.Time, now time.Time, every time.Duration) bool {
	return failed || fetched == nil || now.Sub(*fetched) > 2*every
}

func (u *usage) publish() {
	u.mu.Lock()
	limits, failed, label := u.limits, u.snap.Error != "", u.label
	fetched := u.snap.FetchedAt
	var note stripNote
	if time.Now().Before(u.noteTill) {
		note = u.note
	}
	u.mu.Unlock()
	now := time.Now()
	frame := stripFrameFor(limits, label, now)
	frame.note = note
	if !frame.failed {
		frame.failed, frame.stale = failed, limitsStale(failed, fetched, now, u.every)
	}
	u.mu.Lock()
	u.frame = frame
	u.mu.Unlock()
	u.draw()
	if frame.failed && len(limits) == 0 {
		return
	}
	if s := findLimit(limits, "session"); s != nil {
		u.vars.set("ai_session", strconv.Itoa(round(s.Percent)))
		if s.ResetsAt != nil {
			u.vars.set("ai_session_reset", s.ResetsAt.Local().Format("15:04"))
		}
	}
	if w := findLimit(limits, "weekly_all"); w != nil {
		u.vars.set("ai_week", strconv.Itoa(round(w.Percent)))
		u.vars.set("ai_day", strconv.Itoa(round(dayPct(*w, now))))
		u.vars.set("ai_level", levelOf(dayPct(*w, now)))
		if w.ResetsAt != nil {
			u.vars.set("ai_week_reset", w.ResetsAt.Local().Format("Mon 15:04"))
		}
	}
}

func scopedFor(limits []limit, model string) *limit {
	for i, l := range limits {
		if l.Kind == "weekly_scoped" && model != "" && strings.HasPrefix(strings.ToLower(l.model()), strings.ToLower(model)) {
			return &limits[i]
		}
	}
	return nil
}

func round(n float64) int { return int(math.Floor(n + 0.5)) }

func findLimit(limits []limit, kind string) *limit {
	for i := range limits {
		if limits[i].Kind == kind {
			return &limits[i]
		}
	}
	return nil
}

func levelOf(n float64) string {
	switch {
	case n > 90:
		return "critical"
	case n > 80:
		return "high"
	case n < 50:
		return "low"
	}
	return "normal"
}

func midnight(t time.Time, plusDays int) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day()+plusDays, 0, 0, 0, 0, t.Location())
}

func workdayIndex(start, now time.Time) int {
	count := 0
	for i := 0; i <= 14; i++ {
		d := midnight(start, i)
		if d.After(now) {
			break
		}
		if !d.Before(start) && d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			count++
		}
	}
	return min(workdays, max(1, count))
}

func weekDay(l limit, now time.Time) float64 {
	now = now.Local()
	start := midnight(now, -((int(now.Weekday()) + 6) % 7))
	if l.ResetsAt != nil {
		start = l.ResetsAt.Local().Add(-7 * day)
	}
	return float64(workdayIndex(start, now))
}

func dayPct(l limit, now time.Time) float64 {
	return workdays * l.Percent / weekDay(l, now)
}

func allowedPct(l limit, now time.Time) float64 {
	return 100 * weekDay(l, now) / workdays
}

func current(l *limit, now time.Time) bool {
	return l != nil && (l.ResetsAt == nil || now.Before(*l.ResetsAt))
}

func stripFrameFor(limits []limit, label stripLabel, now time.Time) stripFrame {
	f := stripFrame{label: label, failed: len(limits) == 0}
	if s := findLimit(limits, "session"); current(s, now) {
		f.session = s.Percent
		if s.ResetsAt != nil {
			f.left5h = clockLeft(s.ResetsAt.Sub(now))
		}
	}
	week := findLimit(limits, "weekly_all")
	if current(week, now) {
		f.week, f.allowed, f.weekLvl = week.Percent, allowedPct(*week, now), weekLevelOf(dayPct(*week, now))
		if week.ResetsAt != nil {
			f.leftW = daysLeft(week.ResetsAt.Sub(now))
		}
		if fable := scopedFor(limits, "fable"); current(fable, now) {
			f.fable = min(fable.Percent, week.Percent)
		}
	}
	return f
}

func weekLevelOf(n float64) string {
	switch {
	case n > 100:
		return "over"
	case n > 80:
		return "critical"
	case n < 50:
		return "low"
	}
	return "normal"
}

func clockLeft(d time.Duration) string {
	m := int(math.Ceil(max(0, d.Minutes())))
	return fmt.Sprintf("%d:%02d", m/60, m%60)
}

func daysLeft(d time.Duration) string {
	h := int(math.Ceil(max(0, d.Hours())))
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(math.Ceil(max(0, d.Minutes()))))
	case h < 24:
		return fmt.Sprintf("%dh", h)
	case h%24 == 0:
		return fmt.Sprintf("%dd", h/24)
	}
	return fmt.Sprintf("%dd %dh", h/24, h%24)
}

func stripText(x, size float64, text, ink string) string {
	if text == "" {
		return ""
	}
	return fmt.Sprintf(`<text transform="translate(%.1f %d) rotate(-90)" font-family="%s" font-size="%g" fill="%s">%s</text>`,
		x, stripDrawH-6, stripFont, size, ink, html.EscapeString(text))
}

func aiStripSVG(f stripFrame) string {
	const w, h, w5 = stripDrawW, stripDrawH, stripSessionW
	rect := func(x, width int, pct float64, color string) string {
		bh := float64(h) * min(max(pct, 0), 100) / 100
		return fmt.Sprintf(`<rect x="%d" y="%.1f" width="%d" height="%.1f" fill="%s"/>`, x, h-bh, width, bh, color)
	}
	sessionColor, weekColor := levelColors[levelOf(f.session)], levelColors[f.weekLvl]
	if f.stale {
		sessionColor, weekColor = gray, gray
	}
	if f.weekLvl == "" {
		weekColor = gray
	}
	sessionBar := rect(0, w5, f.session, sessionColor)
	k := min(float64(h)/svgMinWidth, float64(w5)/svgHeight)
	weekBar := rect(w5, w-w5, f.week, weekColor)
	if !f.stale && f.fable > 0 {
		weekBar += rect(w5, w-w5, f.fable, stripFable)
	}
	clip := sessionBar
	if f.weekLvl != "over" {
		clip += weekBar
	}
	label := func(ink string) string {
		fill := "#fff"
		if ink != "" {
			fill = ink
		}
		return fmt.Sprintf(`<g transform="translate(0 %.1f) rotate(-90) scale(%.4f)">%s</g>`,
			(float64(h)+svgMinWidth*k)/2, k, labelMarkup(f.label, f.note, ink)) +
			stripText(float64(svgHeight/2+svgFont*35/100)*k, stripClockFont, f.left5h, fill) +
			stripText(float64(w5)+float64(w-w5)/2+stripDaysFont*0.35, stripDaysFont, f.leftW, fill)
	}
	dot := ""
	if f.failed {
		dot = fmt.Sprintf(`<circle cx="%d" cy="4" r="3" fill="%s"/>`, w-4, red)
	}
	return dataURI(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d">`+
		`<defs><clipPath id="bar">%s</clipPath></defs><rect width="%d" height="%d" fill="#000"/>`+
		`%s%s%s%s<g clip-path="url(#bar)">%s</g>%s</svg>`,
		w, h, clip, w, h,
		rect(w5, w-w5, f.allowed, stripAllowed), weekBar, sessionBar,
		label(""), label("#000"), dot))
}

func labelMarkup(l stripLabel, note stripNote, ink string) string {
	if note.text != "" {
		fill := note.color
		if ink != "" {
			fill = ink
		}
		return fmt.Sprintf(`<text x="%d" y="%d" text-anchor="middle" font-family="%s" font-size="%d" fill="%s">%s</text>`,
			svgMinWidth/2, svgHeight/2+svgFont*35/100, stripFont, svgFont, fill, html.EscapeString(note.text))
	}
	if l.model == "" {
		return ""
	}
	modelColor, dotColor := "#fff", aiStateColors[l.state]
	if l.state == "preview" {
		modelColor = gray
	}
	if ink != "" {
		modelColor, dotColor = ink, ink
	}
	const r, gap = 30, 30
	anchor := svgMinWidth / 2
	y := svgHeight/2 + svgFont*35/100
	var b strings.Builder
	fmt.Fprintf(&b, `<text x="%d" y="%d" text-anchor="end" font-family="%s" font-size="%d" fill="%s">%s</text>`,
		anchor, y, stripFont, svgFont, modelColor, html.EscapeString(strings.ToLower(l.model)))
	effort := strings.ToLower(l.effort)
	if !slices.Contains(aiEffortDots, effort) {
		fmt.Fprintf(&b, `<text x="%d" y="%d" font-family="%s" font-size="%d" font-weight="bold" fill="%s">%s</text>`,
			anchor+2*gap, y, stripFont, svgFont, dotColor, html.EscapeString(effort))
	}
	for i := range slices.Index(aiEffortDots, effort) + 1 {
		fmt.Fprintf(&b, `<circle cx="%d" cy="%d" r="%d" fill="%s"/>`, anchor+2*gap+r+i*(2*r+gap), svgHeight/2, r, dotColor)
	}
	return b.String()
}

func dataURI(svg string) string {
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
}
