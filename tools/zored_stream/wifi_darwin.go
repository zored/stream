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

func wifiCurrent() (wifiLink, error) {
	out, err := exec.Command("/usr/sbin/ipconfig", "getsummary", "en0").Output()
	if err != nil {
		return wifiLink{}, fmt.Errorf("ipconfig getsummary: %w", err)
	}
	return ipconfigLink(string(out))
}

func wifiKnown() ([]string, error) {
	out, err := exec.Command("/usr/bin/security", "dump-keychain", "/Library/Keychains/System.keychain").Output()
	if err != nil {
		return nil, fmt.Errorf("security dump-keychain: %w", err)
	}
	return keychainNetworks(string(out))
}

func runHelper(app string, args ...string) (string, error) {
	f, err := os.CreateTemp("", "zored_wifi-*.tsv")
	if err != nil {
		return "", err
	}
	f.Close()
	defer os.Remove(f.Name())
	if out, err := exec.Command("/usr/bin/open", append([]string{"-g", "-n", "-W", app, "--args", f.Name()}, args...)...).CombinedOutput(); err != nil {
		return "", fmt.Errorf("open %s: %w: %s", app, err, out)
	}
	data, err := os.ReadFile(f.Name())
	return string(data), err
}

func wifiScan(app string) ([]wifiNetwork, error) {
	out, err := runHelper(app)
	if err != nil {
		return nil, err
	}
	return scanNetworks(out)
}

const wifiMenuJoin = `on run argv
	set target to "wifi-network-" & item 1 of argv
	tell application "System Events" to tell process "ControlCenter"
		click (first menu bar item of menu bar 1 whose description is "Control Center")
		delay 0.6
		set tile to missing value
		repeat with e in (UI elements of group 1 of window 1)
			try
				if value of attribute "AXIdentifier" of e is "controlcenter-wifi" then set tile to e
			end try
		end repeat
		if tile is missing value then
			key code 53
			error "no wi-fi tile in Control Center"
		end if
		perform (first action of tile whose name contains "show details")
		repeat 15 times
			delay 0.2
			set nodes to entire contents of window 1
			repeat with e in nodes
				try
					if value of attribute "AXIdentifier" of e is target then
						if item 2 of argv is "find" then
							key code 53
							return "found"
						end if
						click e
						delay 0.3
						key code 53
						return "ok"
					end if
				end try
			end repeat
		end repeat
		key code 53
		error "not in the Wi-Fi menu: " & item 1 of argv
	end tell
end run`

func wifiJoin(ssid string) error {
	out, err := exec.Command("/usr/bin/osascript", "-e", wifiMenuJoin, ssid, "click").CombinedOutput()
	if err != nil {
		return fmt.Errorf("wi-fi join %q: %w: %s", ssid, err, strings.TrimSpace(string(out)))
	}
	return nil
}
