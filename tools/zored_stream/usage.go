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
)

var levelColors = map[string]string{"low": "#99F299", "normal": "#fff", "high": "#FFD94D", "critical": "#FF6666"}

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
	limits, failed := u.limits, u.snap.Error != ""
	u.mu.Unlock()
	if limits == nil {
		u.vars.set("ai_svg", usageSVG([]usageLine{{cells: []usageCell{{"…", "normal"}}}}, failed))
		return
	}
	now := time.Now()
	lines, err := usageLines(limits, now)
	if err != nil {
		log.Printf("usage: %v", err)
		lines = []usageLine{{cells: []usageCell{{"?", "critical"}}}}
	}
	u.vars.set("ai_svg", usageSVG(lines, failed))
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

type usageCell struct{ text, level string }

type usageLine struct {
	cells []usageCell
	dim   bool
}

func usageLines(limits []limit, now time.Time) ([]usageLine, error) {
	week := findLimit(limits, "weekly_all")
	if week == nil {
		return nil, errors.New("no weekly_all")
	}
	var lines []usageLine
	if s := findLimit(limits, "session"); s != nil {
		left := "5h"
		if s.ResetsAt != nil {
			left = durationLeft(s.ResetsAt.Sub(now))
		}
		lines = append(lines, usageLine{cells: []usageCell{{left, levelOf(s.Percent)}}, dim: true})
	}
	lines = append(lines, weekLine(*week, now, false))
	for _, l := range limits {
		if l.Kind == "weekly_scoped" {
			lines = append(lines, weekLine(l, now, true))
			break
		}
	}
	return lines, nil
}

func weekLine(l limit, now time.Time, dim bool) usageLine {
	d := dayPct(l, now)
	return usageLine{cells: []usageCell{{strconv.Itoa(round(d)), levelOf(d)}, {strconv.Itoa(round(l.Percent)), levelOf(l.Percent)}}, dim: dim}
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

func durationLeft(d time.Duration) string {
	total := int(math.Ceil(max(0, d.Minutes())))
	h, m := total/60, total%60
	switch {
	case h == 0:
		return fmt.Sprintf("%dm", m)
	case m == 0:
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}

func usageSVG(lines []usageLine, failed bool) string {
	const size, inset, small, big, stretch = 100, 6, 19, 24, 1.7
	columns := []int{inset, size/2 + 2}
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d">`+
		`<g font-family="Helvetica" font-weight="bold">`, size, size)
	main := false
	for _, line := range lines {
		y, attrs := float64(inset+small*75/100), fmt.Sprintf(`font-size="%d" opacity="0.7"`, small)
		switch {
		case !line.dim:
			main = true
			y, attrs = size/2+big*stretch*36/100, fmt.Sprintf(`font-size="%d" transform="scale(1 %.1f)"`, big, stretch)
		case main:
			y = size - inset
		}
		for j, c := range line.cells {
			ty := y
			if !line.dim {
				ty /= stretch
			}
			fmt.Fprintf(&b, `<text x="%d" y="%.1f" %s fill="%s">%s</text>`,
				columns[j], ty, attrs, levelColors[c.level], html.EscapeString(c.text))
		}
	}
	if failed {
		fmt.Fprintf(&b, `<circle cx="%d" cy="%d" r="3" fill="#FF6666"/>`, size-inset-2, inset)
	}
	b.WriteString(`</g></svg>`)
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(b.String()))
}
