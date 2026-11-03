package main

import (
	"errors"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	wifiHoldFor     = 400 * time.Millisecond
	wifiPickFor     = 5 * time.Second
	wifiJoinAfter   = time.Second
	wifiJoinFor     = 15 * time.Second
	wifiPollEvery   = 3 * time.Second
	wifiProbeEvery  = 15 * time.Second
	wifiProbeURL    = "http://captive.apple.com/hotspot-detect.html"
	wifiOffline     = "offline"
	wifiNameMaxRune = 14
	wifiTileRune    = 10
)

type wifi struct {
	mu       sync.Mutex
	vars     *vars
	strip    func(note stripNote, d time.Duration)
	loading  bool
	cache    []string
	waitJoin bool
	phone    bool
	joining  string
	joinTill time.Time
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

func newWifi(v *vars, strip func(stripNote, time.Duration)) *wifi {
	return &wifi{vars: v, strip: strip, online: true}
}

func (w *wifi) publish() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.publishLocked()
}

func (w *wifi) publishLocked() {
	state, name, phone := "off", w.ssid, w.phone
	switch {
	case time.Now().Before(w.errorTil):
		state = "error"
	case !w.on:
	case w.joining != "" && time.Now().Before(w.joinTill):
		state, name, phone = "joining", w.joining, false
	case w.ssid == "":
		state = "on"
	case !w.online:
		state = "nonet"
	default:
		state = "joined"
	}
	w.vars.set("wifi_svg", wifiSVG(state, name, phone))
}

func (w *wifi) poll() {
	on, err := wifiPower()
	if err != nil {
		log.Print(err)
		return
	}
	var link wifiLink
	if on {
		if link, err = wifiCurrent(); err != nil {
			log.Print(err)
		}
	}
	w.mu.Lock()
	changed := link.ssid != w.ssid
	w.on, w.ssid, w.phone = on, link.ssid, link.phone()
	if link.ssid != "" && link.ssid == w.joining {
		w.joining = ""
	}
	w.publishLocked()
	w.mu.Unlock()
	if changed {
		go w.probe()
	}
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
	w.list = slices.Clone(w.cache)
	if w.ssid != "" && !slices.Contains(w.list, w.ssid) {
		w.list = append([]string{w.ssid}, w.list...)
	}
	w.pick, w.waitJoin = slices.Index(w.list, w.ssid), false
	w.pickTill = time.Now().Add(wifiPickFor)
	w.loading, w.turned = true, false
	w.showLocked()
	w.mu.Unlock()
	go w.scan()
	return nil
}

func (w *wifi) scan() {
	list, err := wifiMenuList()
	if err == nil {
		w.setList(list)
		return
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
	w.cache = list
	if !w.picking() {
		return
	}
	w.list, w.pick = list, keepPick(w.list, w.pick, list)
	w.loading = false
	w.showLocked()
	if w.waitJoin {
		w.waitJoin = false
		w.join = time.AfterFunc(wifiJoinAfter, w.joinPick)
	}
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
	if w.loading {
		w.waitJoin = true
		w.mu.Unlock()
		return
	}
	if w.pick < 0 || w.list[w.pick] == w.ssid {
		w.mu.Unlock()
		return
	}
	ssid := w.list[w.pick]
	w.joining, w.joinTill, w.online = ssid, time.Now().Add(wifiJoinFor), false
	w.publishLocked()
	w.mu.Unlock()
	time.AfterFunc(wifiJoinFor, w.publish)
	if err := wifiJoin(ssid); err != nil {
		log.Print(err)
		w.mu.Lock()
		w.joining = ""
		w.mu.Unlock()
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

type wifiLink struct {
	ssid   string
	router netip.Addr
}

var iphoneHotspot = netip.MustParsePrefix("172.20.10.0/28")

func (l wifiLink) phone() bool {
	return l.router.IsValid() && iphoneHotspot.Contains(l.router)
}

func ipconfigLink(summary string) (wifiLink, error) {
	var l wifiLink
	for line := range strings.Lines(summary) {
		k, v, ok := strings.Cut(strings.TrimSpace(line), " : ")
		switch {
		case !ok:
		case k == "SSID" && v == "<redacted>":
			return wifiLink{}, errors.New("ipconfig: SSID redacted, run sudo ipconfig setverbose 1")
		case k == "SSID":
			l.ssid = v
		case k == "Router" && !l.router.IsValid():
			if a, err := netip.ParseAddr(v); err == nil {
				l.router = a
			}
		}
	}
	return l, nil
}

func keepPick(old []string, pick int, list []string) int {
	if pick >= 0 {
		if i := slices.Index(list, old[pick]); i >= 0 {
			return i
		}
	}
	if len(list) == 0 {
		return -1
	}
	return 0
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

func wifiSVG(state, ssid string, phone bool) string {
	color, fill, text := "#fff", "none", "#fff"
	switch state {
	case "off":
		color = gray
	case "joining":
		text = gray
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
	fmt.Fprintf(&b, `<g transform="translate(0 %d)">`, dy)
	if phone {
		fmt.Fprintf(&b, `<rect x="38" y="30" width="24" height="44" rx="5" fill="%s" stroke="%s" stroke-width="4"/>`, fill, color)
	} else {
		fmt.Fprintf(&b, `<path d="M50 74 L22.4 46.4 A39 39 0 0 1 77.6 46.4 Z" fill="%s" stroke="%s" stroke-width="4" stroke-linejoin="round"/>`, fill, color)
	}
	if state == "nonet" {
		b.WriteString(`<circle cx="80" cy="74" r="6" fill="#FFB000"/>`)
	}
	b.WriteString(`</g>`)
	if ssid != "" {
		fmt.Fprintf(&b, `<text x="50" y="91" text-anchor="middle" font-family="JetBrains Mono" font-size="16" fill="%s">%s</text>`, text, html.EscapeString(clip(ssid, wifiTileRune)))
	}
	b.WriteString(`</svg>`)
	return dataURI(b.String())
}
