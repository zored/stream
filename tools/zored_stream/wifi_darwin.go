package main

/*
#cgo LDFLAGS: -framework CoreWLAN -framework Foundation
#include "wifi_darwin.h"
*/
import "C"

import (
	"errors"
	"fmt"
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

func wifiPreferred() ([]string, error) {
	out, err := exec.Command("/usr/sbin/networksetup", "-listpreferredwirelessnetworks", "en0").Output()
	if err != nil {
		return nil, fmt.Errorf("networksetup preferred: %w", err)
	}
	return preferredNetworks(string(out)), nil
}

func wifiJoin(ssid string) error {
	out, err := exec.Command("/usr/sbin/networksetup", "-setairportnetwork", "en0", ssid).CombinedOutput()
	msg := strings.TrimSpace(string(out))
	if err != nil || msg != "" {
		return fmt.Errorf("wi-fi join %q: %v %s", ssid, err, msg)
	}
	return nil
}
