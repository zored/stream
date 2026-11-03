package main

import (
	"slices"
	"testing"
)

func TestWifiOrder(t *testing.T) {
	got := wifiOrder([]string{"home", "gone", "work"}, []wifiNetwork{
		{ssid: "cafe", rssi: -70}, {ssid: "work", rssi: -40}, {ssid: "home", rssi: -80},
		{ssid: "mall", rssi: -50}, {ssid: "cafe", rssi: -45},
	})
	if want := []string{"home", "work", "cafe", "mall"}; !slices.Equal(got, want) {
		t.Errorf("wifiOrder = %v, want %v", got, want)
	}
}

func TestWifiLabel(t *testing.T) {
	if got := wifiLabel("Момент и ВайФай 5G"); got != "момент и вайф…" {
		t.Errorf("wifiLabel = %q", got)
	}
}

func TestIpconfigSSID(t *testing.T) {
	for summary, want := range map[string]string{
		"<dictionary> {\n  BSSID : aa\n  SSID : Момент и ВайФай\n}": "Момент и ВайФай",
		"<dictionary> {\n  InterfaceType : WiFi\n}":                 "",
	} {
		if got, err := ipconfigSSID(summary); err != nil || got != want {
			t.Errorf("ipconfigSSID = %q, %v, want %q", got, err, want)
		}
	}
	if _, err := ipconfigSSID("  SSID : <redacted>\n"); err == nil {
		t.Error("redacted SSID must fail")
	}
}
