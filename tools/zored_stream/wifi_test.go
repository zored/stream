package main

import (
	"slices"
	"testing"
)

func TestPreferredNetworks(t *testing.T) {
	got := preferredNetworks("Preferred networks on en0:\n\tМомент и ВайФай\n\tzored_smart\n")
	if want := []string{"Момент и ВайФай", "zored_smart"}; !slices.Equal(got, want) {
		t.Errorf("preferredNetworks = %v, want %v", got, want)
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
