package main

import (
	"slices"
	"testing"
	"time"
)

func TestRecoverTargets(t *testing.T) {
	for _, c := range []struct {
		last, fallback, ssid string
		want                 []string
	}{
		{"home", "bo phone", "", []string{"home", "bo phone"}},
		{"", "bo phone", "", []string{"bo phone"}},
		{"bo phone", "bo phone", "", []string{"bo phone"}},
		{"", "", "", nil},
	} {
		if got := recoverTargets(c.last, c.fallback, c.ssid); !slices.Equal(got, c.want) {
			t.Errorf("recoverTargets(%q, %q, %q) = %q, want %q", c.last, c.fallback, c.ssid, got, c.want)
		}
	}
}

func TestRecoverDue(t *testing.T) {
	awake := true
	fresh := func() *wifi {
		awake = true
		return &wifi{fallback: "bo phone", awake: func() bool { return awake }, on: true, lostAt: time.Now().Add(-wifiLostFor)}
	}
	if !fresh().recoverDueLocked() {
		t.Error("awake, no network for 30 s, wi-fi on: want recovery")
	}
	for name, spoil := range map[string]func(w *wifi){
		"sun off":    func(*wifi) { awake = false },
		"wi-fi off":  func(w *wifi) { w.on = false },
		"online":     func(w *wifi) { w.online = true },
		"just lost":  func(w *wifi) { w.lostAt = time.Now() },
		"never lost": func(w *wifi) { w.lostAt = time.Time{} },
		"joined":     func(w *wifi) { w.ssid = "home" },
		"running":    func(w *wifi) { w.recovering = true },
		"picking":    func(w *wifi) { w.pickTill = time.Now().Add(time.Minute) },
		"joining":    func(w *wifi) { w.joining, w.joinTill = "x", time.Now().Add(time.Minute) },
		"backoff":    func(w *wifi) { w.recoverAt = time.Now().Add(time.Minute) },
	} {
		w := fresh()
		spoil(w)
		if w.recoverDueLocked() {
			t.Errorf("%s: recovery must not start", name)
		}
	}
}

func TestWifiLabel(t *testing.T) {
	if got := wifiLabel("Момент и ВайФай 5G"); got != "Момент и ВайФа" {
		t.Errorf("wifiLabel = %q", got)
	}
}

func TestIpconfigLink(t *testing.T) {
	if l := ipconfigLink("<dictionary> {\n  SSID : bo phone\n  Router : 172.20.10.1\n}"); !l.phone() {
		t.Errorf("hotspot: %+v", l)
	}
	if l := ipconfigLink("  SSID : <redacted>\n  Router : 192.168.4.1\n"); l.phone() || !l.router.IsValid() {
		t.Errorf("router: %+v", l)
	}
	if l := ipconfigLink("  SSID : <redacted>\n"); l.router.IsValid() {
		t.Errorf("no router: %+v", l)
	}
}

func TestKeepPick(t *testing.T) {
	list := []string{"a", "b", "c"}
	for _, c := range []struct {
		old  []string
		pick int
		want int
	}{
		{[]string{"x", "c"}, 1, 2},
		{[]string{"x", "gone"}, 1, 0},
		{nil, -1, 0},
	} {
		if got := keepPick(c.old, c.pick, list); got != c.want {
			t.Errorf("keepPick(%v, %d) = %d, want %d", c.old, c.pick, got, c.want)
		}
	}
	if got := keepPick([]string{"a"}, 0, nil); got != -1 {
		t.Errorf("empty list: %d", got)
	}
}

func TestVPN(t *testing.T) {
	vpns, err := parseVPN("barbos-100-5\tConnected\nBarbos_SE\tDisconnected\n\n")
	if err != nil || vpnSummary(vpns) != "up" {
		t.Errorf("up: %v %v", vpns, err)
	}
	if got := vpnSummary(map[string]string{"a": "Disconnected", "b": "Connecting"}); got != "busy" {
		t.Errorf("busy: %s", got)
	}
	if got := vpnSummary(map[string]string{"a": "Disconnected"}); got != "down" {
		t.Errorf("down: %s", got)
	}
	if vpns, err := parseVPN("off\n"); err != nil || vpns != nil {
		t.Errorf("off: %v %v", vpns, err)
	}
}

func TestNextCycle(t *testing.T) {
	cycle := splitList(" bo phone, VK_Team ,zored_smart,")
	if len(cycle) != 3 || cycle[0] != "bo phone" || cycle[1] != "VK_Team" {
		t.Fatalf("splitList: %q", cycle)
	}
	for cur, want := range map[string]string{"bo phone": "VK_Team", "VK_Team": "zored_smart", "zored_smart": "bo phone", "": "bo phone", "zored_5G": "bo phone"} {
		if got := nextCycle(cycle, cur); got != want {
			t.Errorf("nextCycle(%q) = %q, want %q", cur, got, want)
		}
	}
}

func TestTouchCyclesWhilePicking(t *testing.T) {
	w := &wifi{on: true, ssid: "zored_smart", cycle: []string{"bo phone", "VK_Team", "zored_smart"}, strip: func(stripNote, time.Duration) {}}
	w.list, w.pick, w.pickTill = []string{"VK_Team", "zored_smart"}, 1, time.Now().Add(wifiPickFor)
	for _, want := range []string{"bo phone", "VK_Team", "zored_smart", "bo phone"} {
		if err := w.touch(); err != nil {
			t.Fatal(err)
		}
		w.join.Stop()
		if got := w.list[w.pick]; got != want {
			t.Fatalf("pick %q, want %q", got, want)
		}
	}
}
