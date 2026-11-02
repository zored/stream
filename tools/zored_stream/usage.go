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
	"net/http"
	"net/url"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	usageURL        = "https://api.anthropic.com/api/oauth/usage"
	usageKeychain   = "Claude Code-credentials"
	usageMinRefresh = time.Minute
	usageMaxBackoff = time.Hour
	workdays        = 5
	day             = 24 * time.Hour
	sessionHours    = 5
	gray            = "#808080"
	weekShowFor     = 5 * time.Second
	weekHoldMax     = 15 * time.Second
	stripHz         = 30
	stripEasing     = 0.5
	stripDrawW      = 48
	stripDrawH      = 258
	stripTextY      = stripDrawH - 6
)

var (
	aiStateColors = map[string]string{"ok": gray, "pending": gray, "preview": gray, "idle": gray, "other": "#FFB000", "error": "#FF3030"}
	aiEffortDots  = []string{"low", "medium", "high", "extra", "max"}
)

var levelColors = map[string]string{"low": "#fff", "normal": "#fff", "high": "#FFD94D", "critical": "#FF6666"}

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
	mu       sync.Mutex
	every    time.Duration
	http     *http.Client
	vars     *vars
	snap     usageSnapshot
	limits   []limit
	failures int
	wake     chan struct{}
	label    stripLabel
	stripMode
	target  stripFrame
	shown   stripFrame
	animate chan struct{}
}

type stripFrame struct {
	label  stripLabel
	text   string
	failed bool
	pct    float64
	dots   [5]float64
}

type stripLabel struct{ model, effort, state string }

type stripMode struct {
	switching bool
	weekUntil time.Time
}

func newUsage(v *vars, every time.Duration, proxy string) (*usage, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxy != "" {
		u, err := url.Parse(proxy)
		if err != nil {
			return nil, fmt.Errorf("usage proxy: %w", err)
		}
		transport.Proxy = http.ProxyURL(u)
	}
	return &usage{every: every, vars: v, wake: make(chan struct{}, 1), animate: make(chan struct{}, 1),
		http: &http.Client{Timeout: 20 * time.Second, Transport: transport}}, nil
}

func (u *usage) run() {
	for {
		u.fetch()
		u.mu.Lock()
		wait := time.Until(u.snap.NextAt)
		u.mu.Unlock()
		select {
		case <-time.After(wait):
		case <-u.wake:
		}
	}
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
	switching := label.state == "pending"
	u.mu.Lock()
	changed := u.label != label
	now := time.Now()
	switch {
	case label.state == "preview" && u.label.state != "preview":
		u.weekUntil = now.Add(weekHoldMax)
	case u.label.state == "preview" && label.state != "preview" && !switching:
		u.weekUntil = now
	case u.switching && !switching:
		u.weekUntil = now.Add(weekShowFor)
		time.AfterFunc(weekShowFor, u.publish)
	}
	u.label, u.switching = label, switching
	u.mu.Unlock()
	if changed {
		u.publish()
	}
}

func (u *usage) animateStrip() {
	for range u.animate {
		ticker := time.NewTicker(time.Second / stripHz)
		for u.stripStep() {
			<-ticker.C
		}
		ticker.Stop()
	}
}

func (u *usage) stripStep() bool {
	u.mu.Lock()
	moving := ease(&u.shown.pct, u.target.pct, 0.3)
	for i := range u.shown.dots {
		moving = ease(&u.shown.dots[i], u.target.dots[i], 0.02) || moving
	}
	t := u.target
	t.pct, t.dots = u.shown.pct, u.shown.dots
	u.mu.Unlock()
	u.vars.set("ai_strip_svg", aiStripSVG(t))
	return moving
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
		u.failures++
		u.snap.Error = err.Error()
		wait := min(u.every<<u.failures, usageMaxBackoff)
		u.snap.NextAt = now.Add(max(wait, retryAfter))
		log.Printf("usage: %v, next try at %s", err, u.snap.NextAt.Format(time.TimeOnly))
	} else {
		u.failures = 0
		u.snap = usageSnapshot{Limits: raw, FetchedAt: &now, NextAt: now.Add(u.every)}
		u.limits = limits
	}
	u.mu.Unlock()
	u.publish()
}

func (u *usage) get() ([]json.RawMessage, []limit, time.Duration, error) {
	token, err := claudeToken()
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

func claudeToken() (string, error) {
	out, err := exec.Command("/usr/bin/security", "find-generic-password", "-s", usageKeychain, "-w").Output()
	if err != nil {
		return "", fmt.Errorf("keychain: %w", err)
	}
	var creds struct {
		OAuth *struct {
			AccessToken string `json:"accessToken"`
			ExpiresAt   int64  `json:"expiresAt"`
		} `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal(out, &creds); err != nil {
		return "", fmt.Errorf("keychain: %w", err)
	}
	if creds.OAuth == nil || creds.OAuth.AccessToken == "" {
		return "", errors.New("keychain: no claudeAiOauth.accessToken")
	}
	if time.UnixMilli(creds.OAuth.ExpiresAt).Before(time.Now()) {
		return "", errors.New("Claude Code token expired: it refreshes on the next Claude Code use")
	}
	return creds.OAuth.AccessToken, nil
}

func (u *usage) serveJSON(w http.ResponseWriter, _ *http.Request) {
	u.mu.Lock()
	body, err := json.Marshal(u.snap)
	u.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

func (u *usage) publish() {
	u.mu.Lock()
	limits, failed, label, mode := u.limits, u.snap.Error != "", u.label, u.stripMode
	u.mu.Unlock()
	now := time.Now()
	week := scopedFor(limits, label.model)
	if week == nil {
		week = findLimit(limits, "weekly_all")
	}
	frame := stripFrame{label: label, failed: true, dots: dotTargets(label.effort)}
	if week != nil {
		frame.text, frame.pct = stripValue(limits, *week, mode, now)
		frame.failed = failed
	}
	u.mu.Lock()
	u.target = frame
	u.mu.Unlock()
	select {
	case u.animate <- struct{}{}:
	default:
	}
	if week == nil {
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

func dayPct(l limit, now time.Time) float64 {
	now = now.Local()
	start := midnight(now, -((int(now.Weekday()) + 6) % 7))
	if l.ResetsAt != nil {
		start = l.ResetsAt.Local().Add(-7 * day)
	}
	return workdays * l.Percent / float64(workdayIndex(start, now))
}

func stripValue(limits []limit, week limit, mode stripMode, now time.Time) (string, float64) {
	switch {
	case mode.switching:
		return "day", dayPct(week, now)
	case now.Before(mode.weekUntil):
		left := ""
		if week.ResetsAt != nil {
			left = daysLeft(week.ResetsAt.Sub(now))
		}
		return left, week.Percent
	}
	text, pct := clockLeft(sessionHours*time.Hour), 0.0
	if s := findLimit(limits, "session"); s != nil {
		pct = s.Percent
		if s.ResetsAt != nil {
			text = clockLeft(s.ResetsAt.Sub(now))
		}
	}
	return text, pct
}

func dotTargets(effort string) [5]float64 {
	var dots [5]float64
	n := slices.Index(aiEffortDots, strings.ToLower(effort)) + 1
	for i := range n {
		dots[i] = 1
	}
	return dots
}

func aiStripSVG(f stripFrame) string {
	const w, h = stripDrawW, stripDrawH
	barH := h * min(max(f.pct, 0), 100) / 100
	bar := fmt.Sprintf(`x="0" y="%.1f" width="%d" height="%.1f"`, h-barH, w, barH)
	k := min(float64(h)/svgMinWidth, float64(w)/svgHeight)
	content := func(ink string) string {
		fill := gray
		if ink != "" {
			fill = ink
		}
		return fmt.Sprintf(`<g transform="translate(0 %.1f) rotate(-90) scale(%.4f)">%s</g>`+
			`<text x="%d" y="%d" text-anchor="middle" font-family="Helvetica" font-size="17" fill="%s">%s</text>`,
			(float64(h)+svgMinWidth*k)/2, k, labelMarkup(f.label, f.dots, ink), w/2, stripTextY, fill, html.EscapeString(f.text))
	}
	dot := ""
	if f.failed {
		dot = fmt.Sprintf(`<circle cx="%d" cy="4" r="3" fill="#FF6666"/>`, w-4)
	}
	return dataURI(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d">`+
		`<defs><clipPath id="bar"><rect %s/></clipPath></defs><rect width="%d" height="%d" fill="#000"/>`+
		`%s<rect %s fill="#fff"/><g clip-path="url(#bar)">%s</g>%s</svg>`,
		w, h, bar, w, h, content(""), bar, content("#000"), dot))
}

func labelMarkup(l stripLabel, dots [5]float64, ink string) string {
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
	fmt.Fprintf(&b, `<text x="%d" y="%d" text-anchor="end" font-family="Helvetica" font-size="%d" fill="%s">%s</text>`,
		anchor, y, svgFont, modelColor, html.EscapeString(strings.ToLower(l.model)))
	if !slices.Contains(aiEffortDots, strings.ToLower(l.effort)) {
		fmt.Fprintf(&b, `<text x="%d" y="%d" font-family="Helvetica" font-size="%d" font-weight="bold" fill="%s">%s</text>`,
			anchor+2*gap, y, svgFont, dotColor, html.EscapeString(strings.ToLower(l.effort)))
	}
	for i, alpha := range dots {
		if alpha > 0 {
			fmt.Fprintf(&b, `<circle cx="%d" cy="%d" r="%d" fill="%s" opacity="%.2f"/>`,
				anchor+2*gap+r+i*(2*r+gap), svgHeight/2, r, dotColor, alpha)
		}
	}
	return b.String()
}

func clockLeft(d time.Duration) string {
	m := int(math.Ceil(max(0, d.Minutes())))
	return fmt.Sprintf("%d:%02d", m/60, m%60)
}

func daysLeft(d time.Duration) string {
	if d >= day {
		return fmt.Sprintf("%dd", int(d/day))
	}
	return clockLeft(d)
}

func dataURI(svg string) string {
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
}
