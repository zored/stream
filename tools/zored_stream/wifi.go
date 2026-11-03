package main

import (
	"cmp"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"log"
	"maps"
	"net/http"
	"slices"
	"strconv"
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
	wifiTileRune    = 10
)

type wifiNetwork struct {
	ssid string
	rssi int
}

type wifi struct {
	mu       sync.Mutex
	vars     *vars
	app      string
	strip    func(note stripNote, d time.Duration)
	loading  bool
	turned   bool
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

func newWifi(v *vars, app string, strip func(stripNote, time.Duration)) *wifi {
	return &wifi{vars: v, app: app, strip: strip, online: true}
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
	w.vars.set("wifi_svg", wifiSVG(state, w.ssid))
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
	w.loading, w.turned = true, false
	w.showLocked()
	w.mu.Unlock()
	go w.scan()
	return nil
}

func (w *wifi) scan() {
	known, err := wifiKnown()
	if err == nil {
		var found []wifiNetwork
		if found, err = wifiScan(w.app); err == nil {
			w.setList(wifiOrder(known, found))
			return
		}
	}
	log.Print(err)
	w.mu.Lock()
	w.loading = false
	if w.picking() {
		w.showLocked()
	}
	w.mu.Unlock()
	w.fail()
}

func (w *wifi) setList(list []string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.picking() {
		return
	}
	current := ""
	if w.pick >= 0 {
		current = w.list[w.pick]
	}
	if current != "" && !slices.Contains(list, current) {
		list = append([]string{current}, list...)
	}
	w.list, w.pick = list, slices.Index(list, current)
	w.loading = false
	w.showLocked()
}

func wifiOrder(known []string, found []wifiNetwork) []string {
	strongest := map[string]int{}
	for _, n := range found {
		if r, ok := strongest[n.ssid]; !ok || n.rssi > r {
			strongest[n.ssid] = n.rssi
		}
	}
	list := slices.Collect(maps.Keys(strongest))
	isKnown := func(s string) bool { return slices.Contains(known, s) }
	slices.SortFunc(list, func(a, b string) int {
		if ka, kb := isKnown(a), isKnown(b); ka != kb {
			if ka {
				return -1
			}
			return 1
		}
		return cmp.Or(strongest[b]-strongest[a], strings.Compare(a, b))
	})
	return list
}

func scanNetworks(tsv string) ([]wifiNetwork, error) {
	var out []wifiNetwork
	for line := range strings.Lines(tsv) {
		f := strings.Split(strings.TrimSuffix(line, "\n"), "\t")
		if len(f) != 2 {
			return nil, fmt.Errorf("zored_wifi line %q", line)
		}
		if f[0] == "error" {
			return nil, fmt.Errorf("zored_wifi: %s", f[1])
		}
		rssi, err := strconv.Atoi(f[1])
		if err != nil {
			return nil, fmt.Errorf("zored_wifi rssi %q: %w", line, err)
		}
		out = append(out, wifiNetwork{ssid: f[0], rssi: rssi})
	}
	return out, nil
}

func keychainNetworks(dump string) ([]string, error) {
	var out []string
	for _, item := range strings.Split(dump, "keychain: ") {
		if !strings.Contains(item, `"svce"<blob>="AirPort"`) {
			continue
		}
		for line := range strings.Lines(item) {
			_, v, ok := strings.Cut(strings.TrimSpace(line), `"acct"<blob>=`)
			if !ok {
				continue
			}
			if h, ok := strings.CutPrefix(v, "0x"); ok {
				h, _, _ = strings.Cut(h, " ")
				b, err := hex.DecodeString(h)
				if err != nil {
					return nil, fmt.Errorf("keychain acct %q: %w", v, err)
				}
				v = string(b)
			} else {
				v = strings.TrimSuffix(strings.TrimPrefix(v, `"`), `"`)
			}
			out = append(out, v)
		}
	}
	return out, nil
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
	w.turned = true
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
	color := "#fff"
	if w.loading {
		color = "#FFB000"
	}
	w.strip(stripNote{text: wifiLabel(name), color: color, arrow: !w.turned}, time.Until(w.pickTill))
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
		w.strip(stripNote{}, 0)
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
	return clip(name, wifiNameMaxRune)
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		r = append(r[:n-1], '…')
	}
	return string(r)
}

func wifiSVG(state, ssid string) string {
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
	dy := 0
	if ssid != "" {
		dy = -12
	}
	fmt.Fprintf(&b, `<g transform="translate(0 %d)"><path d="M50 74 L22.4 46.4 A39 39 0 0 1 77.6 46.4 Z" fill="%s" stroke="%s" stroke-width="4" stroke-linejoin="round"/>`, dy, fill, color)
	if state == "nonet" {
		b.WriteString(`<circle cx="80" cy="74" r="6" fill="#FFB000"/>`)
	}
	b.WriteString(`</g>`)
	if ssid != "" {
		fmt.Fprintf(&b, `<text x="50" y="91" text-anchor="middle" font-family="JetBrains Mono" font-size="16" fill="#fff">%s</text>`, html.EscapeString(clip(ssid, wifiTileRune)))
	}
	b.WriteString(`</svg>`)
	return dataURI(b.String())
}
