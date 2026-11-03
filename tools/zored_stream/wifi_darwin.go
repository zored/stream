package main

/*
#cgo LDFLAGS: -framework CoreWLAN -framework Foundation
#include <stdlib.h>
#include "wifi_darwin.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"unsafe"
)

func takeString(p *C.char) (string, bool) {
	if p == nil {
		return "", false
	}
	defer C.free(unsafe.Pointer(p))
	return C.GoString(p), true
}

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

func wifiKnown() []string {
	s, _ := takeString(C.wifi_known())
	return strings.FieldsFunc(s, func(r rune) bool { return r == '\n' })
}

func wifiScan() ([]wifiNetwork, error) {
	s, ok := takeString(C.wifi_scan())
	if !ok {
		return nil, errors.New("wi-fi: scan failed")
	}
	var out []wifiNetwork
	for line := range strings.Lines(s) {
		f := strings.Split(strings.TrimSuffix(line, "\n"), "\t")
		if len(f) != 3 {
			return nil, fmt.Errorf("wi-fi scan line %q", line)
		}
		rssi, err := strconv.Atoi(f[1])
		if err != nil {
			return nil, fmt.Errorf("wi-fi scan rssi %q: %w", line, err)
		}
		out = append(out, wifiNetwork{ssid: f[0], rssi: rssi, secure: f[2] == "1"})
	}
	return out, nil
}

func wifiJoin(ssid string) error {
	out, err := exec.Command("/usr/sbin/networksetup", "-setairportnetwork", "en0", ssid).CombinedOutput()
	msg := strings.TrimSpace(string(out))
	if err != nil || msg != "" {
		return fmt.Errorf("wi-fi join %q: %v %s", ssid, err, msg)
	}
	return nil
}
