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
	gray            = "#808080"
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

func (u *usage) focus(model string) {
	u.mu.Lock()
	changed := u.model != model
	u.model = model
	u.mu.Unlock()
	if changed {
		u.publish()
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
	u.mu.Unlock()
	week := scopedFor(limits, model)
	if week == nil {
		week = findLimit(limits, "weekly_all")
	}
	if week == nil {
		u.vars.set("ai_strip_top_svg", "")
		u.vars.set("ai_strip_bottom_svg", "")
		return
	}
	now := time.Now()
	sessionLeft := clockLeft(sessionHours * time.Hour)
	if s := findLimit(limits, "session"); s != nil && s.ResetsAt != nil {
		sessionLeft = clockLeft(s.ResetsAt.Sub(now))
	}
	weekLeft, weekColor := "", gray
	if week.ResetsAt != nil {
		weekLeft, weekColor = daysLeft(week.ResetsAt.Sub(now))
	}
	u.vars.set("ai_strip_top_svg", cellSVG(true, sessionLeft, "#fff", dayPct(*week, now), failed))
	u.vars.set("ai_strip_bottom_svg", cellSVG(false, weekLeft, weekColor, week.Percent, false))
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

func cellSVG(top bool, left, leftColor string, pct float64, failed bool) string {
	timeY, pctY, backY := 84, 64, 44
	if top {
		timeY, pctY, backY = 18, 41, 0
	}
	dot := ""
	if failed {
		dot = `<circle cx="54" cy="6" r="3" fill="#FF6666"/>`
	}
	return dataURI(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 60 90">`+
		`<rect x="0" y="%d" width="60" height="46" fill="#000" opacity="0.75"/>%s`+
		`<g font-family="Helvetica" text-anchor="middle"><text x="30" y="%d" font-size="15" fill="%s">%s</text>`+
		`<text x="30" y="%d" font-size="21" font-weight="bold" fill="%s">%d</text></g></svg>`,
		backY, dot, timeY, leftColor, left, pctY, levelColors[levelOf(pct)], round(pct)))
}

func clockLeft(d time.Duration) string {
	m := int(math.Ceil(max(0, d.Minutes())))
	return fmt.Sprintf("%d:%02d", m/60, m%60)
}

func daysLeft(d time.Duration) (string, string) {
	if d >= day {
		return fmt.Sprintf("%dd", int(d/day)), gray
	}
	return clockLeft(d), "#fff"
}

func dataURI(svg string) string {
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
}
