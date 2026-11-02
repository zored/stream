package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os/exec"
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
	highlightFor    = 2 * time.Second
	gray            = "#808080"
	dark            = "#404040"
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
	model    string
	switched time.Time
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
	return &usage{every: every, vars: v, wake: make(chan struct{}, 1),
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

func (u *usage) focus(model string, switching bool) {
	u.mu.Lock()
	changed := u.model != model || switching
	u.model = model
	if switching {
		u.switched = time.Now()
	}
	u.mu.Unlock()
	if changed {
		u.publish()
		time.AfterFunc(highlightFor, u.publish)
	}
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
	limits, failed, model := u.limits, u.snap.Error != "", u.model
	highlight := time.Since(u.switched) < highlightFor
	u.mu.Unlock()
	if limits == nil {
		u.vars.set("ai_limits_svg", usageSVG(nil, failed))
		return
	}
	now := time.Now()
	bars, err := usageBars(limits, now)
	if err != nil {
		log.Printf("usage: %v", err)
		u.vars.set("ai_limits_svg", usageSVG(nil, true))
		return
	}
	scoped := scopedFor(limits, model)
	colors := barColors(bars, highlight, scoped != nil)
	u.vars.set("ai_limits_svg", usageSVG(bars, failed, colors...))
	week := scoped
	if week == nil {
		week = findLimit(limits, "weekly_all")
	}
	sessionLeft := "5h"
	if s := findLimit(limits, "session"); s != nil && s.ResetsAt != nil {
		sessionLeft = shortDuration(s.ResetsAt.Sub(now))
	}
	weekLeft := ""
	if week.ResetsAt != nil {
		weekLeft = shortDuration(week.ResetsAt.Sub(now))
	}
	u.vars.set("ai_strip_top_svg", cellSVG(sessionLeft, dayPct(*week, now)))
	u.vars.set("ai_strip_bottom_svg", cellSVG(weekLeft, week.Percent))
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
		if l.Kind == "weekly_scoped" && model != "" && strings.HasPrefix(l.model(), model) {
			return &limits[i]
		}
	}
	return nil
}

func barColors(bars []float64, highlight, scoped bool) []string {
	colors := []string{gray, gray, levelColors[levelOf(bars[2])], levelColors[levelOf(bars[3])], gray, gray}
	if !highlight {
		return colors
	}
	if scoped {
		return []string{gray, gray, dark, dark, levelColors[levelOf(bars[4])], levelColors[levelOf(bars[5])]}
	}
	colors[4], colors[5] = dark, dark
	return colors
}

func usageBars(limits []limit, now time.Time) ([]float64, error) {
	week := findLimit(limits, "weekly_all")
	if week == nil {
		return nil, errors.New("no weekly_all")
	}
	left, used := 100.0, 0.0
	if s := findLimit(limits, "session"); s != nil {
		used = s.Percent
		if s.ResetsAt != nil {
			left = max(0, s.ResetsAt.Sub(now).Hours()) / sessionHours * 100
		}
	}
	bars := []float64{left, used, dayPct(*week, now), week.Percent}
	for _, l := range limits {
		if l.Kind == "weekly_scoped" {
			return append(bars, dayPct(l, now), l.Percent), nil
		}
	}
	return append(bars, 0, 0), nil
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

func usageSVG(bars []float64, failed bool, colors ...string) string {
	const size, inset, gap = 100, 6, 1.5
	width := float64(size-2*inset) / 6
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d">`, size, size)
	for i, pct := range bars {
		h := float64(size-2*inset) * min(max(pct, 0), 100) / 100
		fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s"/>`,
			inset+float64(i)*width+gap/2, size-inset-h, width-gap, h, colors[i])
	}
	if failed {
		fmt.Fprintf(&b, `<circle cx="%d" cy="%d" r="3" fill="#FF6666"/>`, size-inset-2, inset+2)
	}
	b.WriteString(`</svg>`)
	return dataURI(b.String())
}

func cellSVG(left string, pct float64) string {
	return dataURI(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 60 90">`+
		`<rect x="0" y="18" width="60" height="54" fill="#000" opacity="0.75"/>`+
		`<g font-family="Helvetica" text-anchor="middle"><text x="30" y="40" font-size="15" fill="%s">%s</text>`+
		`<text x="30" y="65" font-size="21" font-weight="bold" fill="%s">%d%%</text></g></svg>`,
		gray, left, levelColors[levelOf(pct)], round(pct)))
}

func shortDuration(d time.Duration) string {
	m := int(math.Ceil(max(0, d.Minutes())))
	switch {
	case m >= 24*60:
		return fmt.Sprintf("%dd %dh", m/(24*60), m%(24*60)/60)
	case m >= 60:
		return fmt.Sprintf("%dh %dm", m/60, m%60)
	}
	return fmt.Sprintf("%dm", m)
}

func dataURI(svg string) string {
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
}
