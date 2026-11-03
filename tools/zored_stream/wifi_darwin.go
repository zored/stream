package main

/*
#cgo LDFLAGS: -framework CoreWLAN -framework Foundation
#include "wifi_darwin.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func wifiPower() (bool, error) {
	switch C.wifi_power() {
	case 1:
		return true, nil
	case 0:
		return false, nil
	}
	return false, errors.New("wi-fi: no interface")
}

func wifiSetPower(on bool) error {
	flag := 0
	if on {
		flag = 1
	}
	if C.wifi_set_power(C.int(flag)) == 0 {
		return fmt.Errorf("wi-fi: set power %v failed", on)
	}
	return nil
}

func wifiSSID() (string, error) {
	out, err := exec.Command("/usr/sbin/ipconfig", "getsummary", "en0").Output()
	if err != nil {
		return "", fmt.Errorf("ipconfig getsummary: %w", err)
	}
	return ipconfigSSID(string(out))
}

func wifiKnown() ([]string, error) {
	out, err := exec.Command("/usr/bin/security", "dump-keychain", "/Library/Keychains/System.keychain").Output()
	if err != nil {
		return nil, fmt.Errorf("security dump-keychain: %w", err)
	}
	return keychainNetworks(string(out))
}

func wifiScan(app string) ([]wifiNetwork, error) {
	f, err := os.CreateTemp("", "zored_wifi-*.tsv")
	if err != nil {
		return nil, err
	}
	f.Close()
	defer os.Remove(f.Name())
	if out, err := exec.Command("/usr/bin/open", "-g", "-n", "-W", app, "--args", f.Name()).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("open %s: %w: %s", app, err, out)
	}
	data, err := os.ReadFile(f.Name())
	if err != nil {
		return nil, err
	}
	return scanNetworks(string(data))
}

func wifiJoin(ssid string) error {
	out, err := exec.Command("/usr/sbin/networksetup", "-setairportnetwork", "en0", ssid).CombinedOutput()
	msg := strings.TrimSpace(string(out))
	if err != nil || msg != "" {
		return fmt.Errorf("wi-fi join %q: %v %s", ssid, err, msg)
	}
	return nil
}
