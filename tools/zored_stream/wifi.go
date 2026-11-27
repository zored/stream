package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	wifiPickFor     = 5 * time.Second
	wifiJoinAfter   = time.Second
	wifiJoinFor     = 15 * time.Second
	vpnAfter        = time.Second
	vpnRereadEvery  = 30 * time.Second
	wifiPollEvery   = 3 * time.Second
	wifiProbeEvery  = 15 * time.Second
	wifiLostFor     = 30 * time.Second
	wifiRecoverWait = time.Minute
	wifiProbeURL    = "http://captive.apple.com/hotspot-detect.html"
	wifiOffline     = "offline"
	wifiNameMaxRune = 14
)

type wifi struct {
	mu       sync.Mutex
	vars     *vars
	strip    func(note stripNote, d time.Duration)
	loading  bool
	cache    []string
	vpn      string
	tunnel   bool
	vpnAt    time.Time
	vpnName  string
	waitJoin bool
	phone    bool
	joining  string
	joinTill time.Time
	on       bool
	ssid     string
	online   bool
	pickTill time.Time
	list     []string
	pick     int
	join     *time.Timer
	errorTil time.Time

	awake      func() bool
	fallback   string
	last       string
	lostAt     time.Time
	recovering bool
	recoverAt  time.Time
	vpnOwed    bool
	cycle      []string
}

func newWifi(v *vars, vpnName, fallback string, cycle []string, awake func() bool, strip func(stripNote, time.Duration)) *wifi {
	return &wifi{vars: v, vpnName: vpnName, fallback: fallback, cycle: cycle, awake: awake, strip: strip, online: true}
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
	w.vars.set("wifi_svg", wifiSVG(state, name, phone, w.vpn))
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
	tunnel, err := vpnTunnel()
	if err != nil {
		log.Print(err)
	}
	w.mu.Lock()
	stale := tunnel != w.tunnel || w.vpn == "busy" || time.Since(w.vpnAt) >= vpnRereadEvery
	w.mu.Unlock()
	vpn := ""
	if stale {
		if vpn, err = vpnStatus(); err != nil {
			log.Print(err)
		}
	}
	w.mu.Lock()
	if stale {
		w.vpn, w.tunnel, w.vpnAt = vpn, tunnel, time.Now()
	}
	changed := link.ssid != w.ssid
	w.on, w.ssid, w.phone = on, link.ssid, link.phone()
	switch {
	case !on || link.ssid != "":
		w.lostAt = time.Time{}
	case w.lostAt.IsZero():
		w.lostAt = time.Now()
	}
	lost := w.recoverDueLocked()
	w.recovering = w.recovering || lost
	if link.ssid != "" && link.ssid == w.joining {
		w.joining = ""
	}
	w.publishLocked()
	w.mu.Unlock()
	if changed {
		go w.probe()
	}
	if lost {
		go w.recover()
	}
}

var wifiOnline = func() bool {
	c := &http.Client{Timeout: 5 * time.Second}
	resp, err := c.Get(wifiProbeURL)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (w *wifi) probe() bool {
	online := wifiOnline()
	w.mu.Lock()
	w.online = online
	restore, ssid := false, w.ssid
	if online {
		if w.ssid != "" && w.ssid != w.fallback {
			w.last = w.ssid
		}
		if w.vpnOwed {
			w.vpnOwed, restore = false, true
		}
	}
	w.publishLocked()
	w.mu.Unlock()
	if restore {
		go w.restoreVPN(ssid)
	}
	return online
}

func (w *wifi) recoverDueLocked() bool {
	now := time.Now()
	return w.on && w.ssid == "" && !w.online && !w.lostAt.IsZero() && now.Sub(w.lostAt) >= wifiLostFor && !w.recovering && !w.picking() &&
		!(w.joining != "" && now.Before(w.joinTill)) && !now.Before(w.recoverAt) &&
		len(recoverTargets(w.last, w.fallback, w.ssid)) > 0 && w.awake()
}

func recoverTargets(last, fallback, ssid string) []string {
	var targets []string
	for _, t := range []string{last, fallback} {
		if t != "" && t != ssid && !slices.Contains(targets, t) {
			targets = append(targets, t)
		}
	}
	return targets
}

// recover runs with the keep-awake session on once Wi-Fi has had no network for wifiLostFor: the last network, then the fallback hotspot.
func (w *wifi) recover() {
	defer func() {
		w.mu.Lock()
		w.recovering, w.recoverAt, w.joining = false, time.Now().Add(wifiRecoverWait), ""
		w.publishLocked()
		w.mu.Unlock()
	}()
	w.mu.Lock()
	targets := recoverTargets(w.last, w.fallback, w.ssid)
	w.mu.Unlock()
	log.Printf("wi-fi recover: no network for %v, trying %q", wifiLostFor, targets)
	if w.vpnName != "" {
		if err := vpnDisconnectAll(); err != nil {
			log.Print(err)
			return
		}
		w.mu.Lock()
		w.vpnOwed = true
		w.mu.Unlock()
	}
	for _, ssid := range targets {
		w.mu.Lock()
		w.joining, w.joinTill = ssid, time.Now().Add(wifiJoinFor+wifiJoinFor)
		w.publishLocked()
		w.mu.Unlock()
		if err := wifiJoin(ssid); err != nil {
			log.Printf("wi-fi recover: %v", err)
			continue
		}
		if w.waitOnline(ssid) {
			log.Printf("wi-fi recover: online through %q", ssid)
			return
		}
		log.Printf("wi-fi recover: %q has no internet", ssid)
	}
	w.fail()
}

func (w *wifi) waitOnline(ssid string) bool {
	for deadline := time.Now().Add(wifiJoinFor); time.Now().Before(deadline); {
		time.Sleep(time.Second)
		if l, err := wifiCurrent(); err != nil || l.ssid != ssid || !l.router.IsValid() {
			continue
		}
		w.poll()
		if w.probe() {
			return true
		}
	}
	return false
}

func (w *wifi) power() error {
	w.mu.Lock()
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

func (w *wifi) touch() error {
	w.mu.Lock()
	if !w.on {
		w.mu.Unlock()
		return nil
	}
	if w.picking() && len(w.cycle) > 0 {
		w.cycleLocked()
		w.mu.Unlock()
		return nil
	}
	w.list = slices.Clone(w.cache)
	if w.ssid != "" && !slices.Contains(w.list, w.ssid) {
		w.list = append([]string{w.ssid}, w.list...)
	}
	w.pick, w.waitJoin = slices.Index(w.list, w.ssid), false
	w.pickTill = time.Now().Add(wifiPickFor)
	w.loading = true
	w.showLocked()
	w.mu.Unlock()
	go w.scan()
	return nil
}

func (w *wifi) cycleLocked() {
	cur := w.ssid
	if w.pick >= 0 {
		cur = w.list[w.pick]
	}
	next := nextCycle(w.cycle, cur)
	if !slices.Contains(w.list, next) {
		w.list = append(slices.Clone(w.list), next)
	}
	w.pick = slices.Index(w.list, next)
	w.pickTill = time.Now().Add(wifiPickFor)
	if w.join != nil {
		w.join.Stop()
	}
	w.join = time.AfterFunc(wifiJoinAfter, w.joinPick)
	w.showLocked()
}

func nextCycle(cycle []string, cur string) string {
	i := slices.Index(cycle, cur)
	return cycle[(i+1)%len(cycle)]
}

func (w *wifi) scan() {
	list, err := wifiList(false)
	if err == nil {
		w.setList(list)
		if list, err = wifiList(true); err == nil {
			w.refreshList(list)
			return
		}
		log.Print(err)
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

func (w *wifi) refreshList(list []string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.cache = list
	if !w.picking() || w.pick >= 0 && !slices.Contains(list, w.list[w.pick]) {
		return
	}
	w.list, w.pick = list, keepPick(w.list, w.pick, list)
	w.showLocked()
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

func (w *wifi) capturing() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.picking()
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
	color := "#fff"
	if w.loading {
		color = amber
	}
	w.strip(stripNote{text: wifiLabel(name), color: color}, time.Until(w.pickTill))
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
	if err := w.switchNetwork(ssid); err != nil {
		log.Print(err)
		w.mu.Lock()
		w.joining = ""
		w.mu.Unlock()
		w.fail()
		return
	}
	w.poll()
}

func (w *wifi) close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closePick()
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

func ipconfigLink(summary string) wifiLink {
	var l wifiLink
	for line := range strings.Lines(summary) {
		k, v, ok := strings.Cut(strings.TrimSpace(line), " : ")
		switch {
		case !ok:
		case k == "Router" && !l.router.IsValid():
			if a, err := netip.ParseAddr(v); err == nil {
				l.router = a
			}
		}
	}
	return l
}

func (w *wifi) switchNetwork(ssid string) error {
	if w.vpnName != "" {
		if err := vpnDisconnectAll(); err != nil {
			return err
		}
		w.mu.Lock()
		w.vpnOwed = false
		w.mu.Unlock()
		defer func() { go w.restoreVPN(ssid) }()
	}
	return wifiJoin(ssid)
}

func (w *wifi) restoreVPN(ssid string) {
	deadline := time.Now().Add(wifiJoinFor)
	for {
		l, err := wifiCurrent()
		if err == nil && l.ssid == ssid && l.router.IsValid() {
			break
		}
		if time.Now().After(deadline) {
			log.Printf("viscosity: %q has no link after %v, connecting anyway", ssid, wifiJoinFor)
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	time.Sleep(vpnAfter)
	for attempt := 1; attempt <= 2; attempt++ {
		if err := vpnConnect(w.vpnName); err != nil {
			log.Print(err)
		}
		for range 20 {
			time.Sleep(time.Second)
			vpns, err := vpnConnections()
			if err == nil && vpns[w.vpnName] == "Connected" {
				w.poll()
				return
			}
		}
		log.Printf("viscosity: %s not connected after attempt %d", w.vpnName, attempt)
	}
	w.poll()
}

// vpnTunnel is a fork-free hint: a Viscosity (OpenVPN) link is a utun with IPv4, system utuns carry only IPv6.
func vpnTunnel() (bool, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return false, fmt.Errorf("vpn tunnel: %w", err)
	}
	for _, i := range ifs {
		if !strings.HasPrefix(i.Name, "utun") {
			continue
		}
		addrs, err := i.Addrs()
		if err != nil {
			return false, fmt.Errorf("vpn tunnel %s: %w", i.Name, err)
		}
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil {
				return true, nil
			}
		}
	}
	return false, nil
}

func vpnStatus() (string, error) {
	vpns, err := vpnConnections()
	if err != nil || vpns == nil {
		return "", err
	}
	return vpnSummary(vpns), nil
}

func vpnSummary(vpns map[string]string) string {
	summary := "down"
	for _, state := range vpns {
		switch state {
		case "Connected":
			return "up"
		case "Disconnected":
		default:
			summary = "busy"
		}
	}
	return summary
}

func parseVPN(out string) (map[string]string, error) {
	if strings.TrimSpace(out) == "off" {
		return nil, nil
	}
	vpns := map[string]string{}
	for line := range strings.Lines(out) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		name, state, ok := strings.Cut(strings.TrimSuffix(line, "\n"), "\t")
		if !ok {
			return nil, fmt.Errorf("viscosity line %q", line)
		}
		vpns[name] = state
	}
	return vpns, nil
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
		r = r[:n]
	}
	return string(r)
}

func wifiSVG(state, ssid string, phone bool, vpn string) string {
	color, fill, text := "#fff", "none", "#fff"
	switch state {
	case "off":
		color = gray
	case "joining":
		text = gray
	case "error":
		color = red
	case "joined", "nonet":
		fill = "#fff"
	}
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">`)
	cy := iconCY
	name := "wifi-1"
	switch {
	case phone && fill != "none":
		name = "phone-fill"
	case phone:
		name = "phone"
	case state == "off" || state == "error":
		name = "wifi-off"
	case fill != "none":
		name = "wifi"
	}
	b.WriteString(iconAt(name, color, 50, cy, iconSize))
	if state == "nonet" {
		fmt.Fprintf(&b, `<circle cx="78" cy="%g" r="5" fill="%s"/>`, cy+22, amber)
	}
	if c, ok := map[string]string{"up": "#fff", "busy": amber, "down": red}[vpn]; ok {
		eye := "eye-fill"
		if vpn == "down" {
			eye = "eye-slash-fill"
		}
		b.WriteString(iconAt(eye, c, 84, 14, 22))
	}
	if ssid != "" {
		b.WriteString(strings.Replace(captionText(ssid, stripFont), `fill="#fff"`, `fill="`+text+`"`, 1))
	}
	b.WriteString(`</svg>`)
	return dataURI(b.String())
}

func splitList(s string) []string {
	var out []string
	for _, f := range strings.Split(s, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}
