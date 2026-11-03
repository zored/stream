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
