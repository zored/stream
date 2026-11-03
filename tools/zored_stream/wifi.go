package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	wifiHoldFor     = 400 * time.Millisecond
	wifiPickFor     = 5 * time.Second
	wifiJoinAfter   = 2 * time.Second
	wifiPollEvery   = 3 * time.Second
	wifiProbeEvery  = 15 * time.Second
	wifiProbeURL    = "http://captive.apple.com/hotspot-detect.html"
	wifiOffline     = "offline"
	wifiNameMaxRune = 14
)

type wifiNetwork struct {
	ssid   string
	rssi   int
	secure bool
}

type wifi struct {
	mu       sync.Mutex
	vars     *vars
	strip    func(note string, d time.Duration)
	on       bool
	ssid     string
	online   bool
	downAt   time.Time
	pickTill time.Time
	list     []string
	pick     int
	join     *time.Timer
	errorTil time.Time
}

func newWifi(v *vars, strip func(string, time.Duration)) *wifi {
	return &wifi{vars: v, strip: strip, online: true}
}

func (w *wifi) publish() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.publishLocked()
}

func (w *wifi) publishLocked() {
	state := "off"
	switch {
	case time.Now().Before(w.errorTil):
		state = "error"
	case !w.on:
	case w.ssid == "":
		state = "on"
	case !w.online:
		state = "nonet"
	default:
		state = "joined"
	}
	w.vars.set("wifi_svg", wifiSVG(state))
}

func (w *wifi) poll() {
	on, err := wifiPower()
	if err != nil {
		log.Print(err)
		return
	}
	ssid := ""
	if on {
		if ssid, err = wifiSSID(); err != nil {
			log.Print(err)
		}
	}
	w.mu.Lock()
	w.on, w.ssid = on, ssid
	w.publishLocked()
	w.mu.Unlock()
}

func (w *wifi) probe() {
	c := &http.Client{Timeout: 5 * time.Second}
	resp, err := c.Get(wifiProbeURL)
	online := err == nil && resp.StatusCode == http.StatusOK
	if err == nil {
		resp.Body.Close()
	}
	w.mu.Lock()
	w.online = online
	w.publishLocked()
	w.mu.Unlock()
}

func (w *wifi) down() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.downAt = time.Now()
	return nil
}

func (w *wifi) up() error {
	w.mu.Lock()
	if w.downAt.IsZero() {
		w.mu.Unlock()
		return errors.New("release without press")
	}
	held := time.Since(w.downAt)
	w.downAt = time.Time{}
	if held >= wifiHoldFor {
		on := !w.on
		w.closePick()
		w.mu.Unlock()
		if err := wifiSetPower(on); err != nil {
			w.fail()
			return err
		}
		w.poll()
		return nil
	}
	if !w.on {
		w.mu.Unlock()
		return nil
	}
	w.list, w.pick = nil, -1
	if w.ssid != "" {
		w.list, w.pick = []string{w.ssid}, 0
	}
	w.pickTill = time.Now().Add(wifiPickFor)
	w.showLocked()
	w.mu.Unlock()
	go w.scan()
	return nil
}

func (w *wifi) scan() {
	found, err := wifiScan()
	if err != nil {
		log.Print(err)
		w.fail()
		return
	}
	list := wifiOrder(wifiKnown(), found)
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.picking() {
		return
	}
	current := ""
	if w.pick >= 0 {
		current = w.list[w.pick]
	}
	w.list, w.pick = list, slices.Index(list, current)
}

func wifiOrder(known []string, found []wifiNetwork) []string {
	strongest := map[string]int{}
	for _, n := range found {
		if r, ok := strongest[n.ssid]; !ok || n.rssi > r {
			strongest[n.ssid] = n.rssi
		}
	}
	var out []string
	for _, k := range known {
		if _, ok := strongest[k]; ok && !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	var rest []string
	for s := range strongest {
		if !slices.Contains(out, s) {
			rest = append(rest, s)
		}
	}
	slices.SortFunc(rest, func(a, b string) int { return strongest[b] - strongest[a] })
	return append(out, rest...)
}

func (w *wifi) picking() bool {
	return time.Now().Before(w.pickTill)
}

func (w *wifi) turn(direction int) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.picking() {
		return false
	}
	if len(w.list) > 0 {
		w.pick = (max(w.pick, 0) + direction + len(w.list)) % len(w.list)
	}
	w.pickTill = time.Now().Add(wifiPickFor)
	if w.join != nil {
		w.join.Stop()
	}
	w.join = time.AfterFunc(wifiJoinAfter, w.joinPick)
	w.showLocked()
	return true
}

func (w *wifi) showLocked() {
	name := wifiOffline
	if w.pick >= 0 {
		name = w.list[w.pick]
	}
	w.strip(wifiLabel(name), time.Until(w.pickTill))
}

func (w *wifi) joinPick() {
	w.mu.Lock()
	w.join = nil
	if w.pick < 0 || w.list[w.pick] == w.ssid {
		w.mu.Unlock()
		return
	}
	ssid := w.list[w.pick]
	w.mu.Unlock()
	if err := wifiJoin(ssid); err != nil {
		log.Print(err)
		w.fail()
		return
	}
	w.poll()
}

func (w *wifi) closePick() {
	if w.join != nil {
		w.join.Stop()
		w.join = nil
	}
	if w.picking() {
		w.pickTill = time.Time{}
		w.strip("", 0)
	}
}

func (w *wifi) fail() {
	w.mu.Lock()
	w.errorTil = time.Now().Add(3 * time.Second)
	w.publishLocked()
	w.mu.Unlock()
	time.AfterFunc(3*time.Second, w.publish)
}

func ipconfigSSID(summary string) (string, error) {
	for line := range strings.Lines(summary) {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), " : "); ok && k == "SSID" {
			if v == "<redacted>" {
				return "", errors.New("ipconfig: SSID redacted, run sudo ipconfig setverbose 1")
			}
			return v, nil
		}
	}
	return "", nil
}

func wifiLabel(name string) string {
	r := []rune(strings.ToLower(name))
	if len(r) > wifiNameMaxRune {
		r = append(r[:wifiNameMaxRune-1], '…')
	}
	return string(r)
}

func wifiSVG(state string) string {
	color, fill := "#fff", "none"
	switch state {
	case "off":
		color = gray
	case "error":
		color = "#FF3030"
	case "joined", "nonet":
		fill = "#fff"
	}
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">`)
	fmt.Fprintf(&b, `<path d="M50 74 L22.4 46.4 A39 39 0 0 1 77.6 46.4 Z" fill="%s" stroke="%s" stroke-width="4" stroke-linejoin="round"/>`, fill, color)
	if state == "nonet" {
		b.WriteString(`<circle cx="80" cy="74" r="6" fill="#FFB000"/>`)
	}
	b.WriteString(`</svg>`)
	return dataURI(b.String())
}
