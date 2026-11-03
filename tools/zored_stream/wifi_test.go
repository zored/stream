package main

import (
	"testing"
)

func TestWifiLabel(t *testing.T) {
	if got := wifiLabel("Момент и ВайФай 5G"); got != "Момент и ВайФ…" {
		t.Errorf("wifiLabel = %q", got)
	}
}

func TestIpconfigLink(t *testing.T) {
	l, err := ipconfigLink("<dictionary> {\n  SSID : bo phone\n  Router : 172.20.10.1\n}")
	if err != nil || l.ssid != "bo phone" || !l.phone() {
		t.Errorf("hotspot: %+v, %v", l, err)
	}
	l, err = ipconfigLink("  SSID : Момент и ВайФай\n  Router : 192.168.4.1\n")
	if err != nil || l.ssid != "Момент и ВайФай" || l.phone() {
		t.Errorf("router: %+v, %v", l, err)
	}
	if _, err := ipconfigLink("  SSID : <redacted>\n"); err == nil {
		t.Error("redacted SSID must fail")
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
