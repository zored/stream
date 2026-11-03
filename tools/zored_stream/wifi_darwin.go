package main

/*
#cgo LDFLAGS: -framework CoreWLAN -framework Foundation
#include "wifi_darwin.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"
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

const wifiMenu = `on run argv
	set mode to item 1 of argv
	tell application "System Events" to tell process "ControlCenter"
		set items_ to menu bar items of menu bar 1
		repeat 25 times
			if (count of windows) is 0 then exit repeat
			delay 0.1
		end repeat
		repeat with i from 1 to count of items_
			if description of item i of items_ starts with "Control Center" then
				perform action "AXPress" of item i of items_
				exit repeat
			end if
		end repeat
		repeat 10 times
			if (count of windows) > 0 then exit repeat
			delay 0.1
		end repeat
		if (count of windows) is 0 then error "Control Center did not open"
		delay 0.3
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
		set previous to {}
		repeat 20 times
			delay 0.2
			set nodes to {}
			try
				set nodes to entire contents of window 1
			end try
			set found to {}
			repeat with e in nodes
				try
					set id_ to value of attribute "AXIdentifier" of e
					if id_ starts with "wifi-network-" then
						if mode is "join" and id_ is "wifi-network-" & item 2 of argv then
							click e
							delay 0.3
							key code 53
							return "ok"
						end if
						set end of found to text 14 thru -1 of id_
					end if
				end try
			end repeat
			if mode is "list" and found is not {} and found is previous then
				key code 53
				set AppleScript's text item delimiters to linefeed
				return found as text
			end if
			set previous to found
		end repeat
		key code 53
		if mode is "list" then return ""
		error "not in the Wi-Fi menu: " & item 2 of argv
	end tell
end run`

var wifiMenuMu sync.Mutex

func wifiMenuRun(args ...string) (string, error) {
	wifiMenuMu.Lock()
	defer wifiMenuMu.Unlock()
	out, err := exec.Command("/usr/bin/osascript", append([]string{"-e", wifiMenu}, args...)...).CombinedOutput()
	if err != nil {
		log.Printf("wi-fi menu %q retry: %s", args, strings.TrimSpace(string(out)))
		time.Sleep(time.Second)
		out, err = exec.Command("/usr/bin/osascript", append([]string{"-e", wifiMenu}, args...)...).CombinedOutput()
	}
	if err != nil {
		return "", fmt.Errorf("wi-fi menu %q: %w: %s", args, err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSuffix(string(out), "\n"), nil
}

func wifiMenuList() ([]string, error) {
	out, err := wifiMenuRun("list")
	if err != nil || out == "" {
		return nil, err
	}
	return strings.Split(out, "\n"), nil
}

func wifiJoin(ssid string) error {
	_, err := wifiMenuRun("join", ssid)
	return err
}

func wifiDisassociate() {
	C.wifi_disassociate()
}

const viscosityState = `if application "Viscosity" is running then
	tell application "Viscosity"
		set out to ""
		repeat with c in connections
			set out to out & (name of c) & tab & (state of c) & linefeed
		end repeat
		return out
	end tell
end if
return "off"`

func vpnConnections() (map[string]string, error) {
	out, err := exec.Command("/usr/bin/osascript", "-e", viscosityState).Output()
	if err != nil {
		return nil, fmt.Errorf("viscosity state: %w", err)
	}
	return parseVPN(string(out))
}

func vpnRun(script string, args ...string) error {
	out, err := exec.Command("/usr/bin/osascript", append([]string{"-e", script}, args...)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("viscosity: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func vpnDisconnectAll() error {
	return vpnRun(`tell application "Viscosity" to disconnectall`)
}

func vpnConnect(name string) error {
	return vpnRun("on run argv\n\ttell application \"Viscosity\" to connect (item 1 of argv)\nend run", name)
}
