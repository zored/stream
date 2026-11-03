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
	if want := []string{"work", "home", "cafe", "mall"}; !slices.Equal(got, want) {
		t.Errorf("wifiOrder = %v, want %v", got, want)
	}
}

func TestScanNetworks(t *testing.T) {
	got, err := scanNetworks("zored\t-49\nMTS_GPON_0E69\t-48\n")
	if want := []wifiNetwork{{"zored", -49}, {"MTS_GPON_0E69", -48}}; err != nil || !slices.Equal(got, want) {
		t.Errorf("scanNetworks = %v, %v", got, err)
	}
	if _, err := scanNetworks("error\tlocation denied\n"); err == nil {
		t.Error("helper error must fail")
	}
}

func TestKeychainNetworks(t *testing.T) {
	dump := "keychain: \"/Library/Keychains/System.keychain\"\nattributes:\n    \"acct\"<blob>=\"zored_5G\"\n    \"svce\"<blob>=\"AirPort\"\n" +
		"keychain: \"/Library/Keychains/System.keychain\"\nattributes:\n    \"acct\"<blob>=0x6950686F6E652028D095D0B2D0B3D0B5D0BDD0B8D18F29  \"iPhone\"\n    \"svce\"<blob>=\"AirPort\"\n" +
		"keychain: \"/Library/Keychains/System.keychain\"\nattributes:\n    \"acct\"<blob>=\"root\"\n    \"svce\"<blob>=\"other\"\n"
	got, err := keychainNetworks(dump)
	if want := []string{"zored_5G", "iPhone (Евгения)"}; err != nil || !slices.Equal(got, want) {
		t.Errorf("keychainNetworks = %q, %v", got, err)
	}
}

func TestWifiLabel(t *testing.T) {
	if got := wifiLabel("Момент и ВайФай 5G"); got != "Момент и ВайФ…" {
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
