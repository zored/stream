package main

/*
#cgo LDFLAGS: -framework IOKit -framework CoreFoundation -framework ApplicationServices
#include <IOKit/pwr_mgt/IOPMLib.h>
#include <ApplicationServices/ApplicationServices.h>

static IOReturn sleepNow(void) {
	io_connect_t port = IOPMFindPowerManagement(MACH_PORT_NULL);
	if (port == MACH_PORT_NULL) {
		return kIOReturnNotFound;
	}
	IOReturn r = IOPMSleepSystem(port);
	IOServiceClose(port);
	return r;
}

static IOPMAssertionID activityID = kIOPMNullAssertionID;

static IOReturn userActive(void) {
	CGEventRef here = CGEventCreate(NULL);
	CGEventRef move = CGEventCreateMouseEvent(NULL, kCGEventMouseMoved, CGEventGetLocation(here), kCGMouseButtonLeft);
	CGEventPost(kCGHIDEventTap, move);
	CFRelease(move);
	CFRelease(here);
	IOPMAssertionID previous = activityID;
	activityID = kIOPMNullAssertionID;
	IOReturn r = IOPMAssertionDeclareUserActivity(CFSTR("zored_stream controller"), kIOPMUserActiveLocal, &activityID);
	if (previous != kIOPMNullAssertionID) IOPMAssertionRelease(previous);
	return r;
}

static int screensAsleep(void) {
	CGDirectDisplayID ids[16];
	uint32_t n = 0;
	if (CGGetOnlineDisplayList(16, ids, &n) != kCGErrorSuccess) {
		return -1;
	}
	for (uint32_t i = 0; i < n; i++) {
		if (!CGDisplayIsAsleep(ids[i])) return 0;
	}
	return 1;
}

static int sessionLocked(void) {
	CFDictionaryRef session = CGSessionCopyCurrentDictionary();
	if (!session) return -1;
	CFBooleanRef locked = CFDictionaryGetValue(session, CFSTR("CGSSessionScreenIsLocked"));
	int r = locked && CFBooleanGetValue(locked);
	CFRelease(session);
	return r;
}

static int amphAsserted(void) {
	CFDictionaryRef byPid = NULL;
	if (IOPMCopyAssertionsByProcess(&byPid) != kIOReturnSuccess || !byPid) {
		return -1;
	}
	CFIndex n = CFDictionaryGetCount(byPid);
	const void *lists[n];
	CFDictionaryGetKeysAndValues(byPid, NULL, lists);
	int found = 0;
	for (CFIndex i = 0; i < n && !found; i++) {
		CFArrayRef list = lists[i];
		for (CFIndex j = 0; j < CFArrayGetCount(list) && !found; j++) {
			CFStringRef name = CFDictionaryGetValue(CFArrayGetValueAtIndex(list, j), kIOPMAssertionNameKey);
			found = name && CFStringHasPrefix(name, CFSTR("Amphetamine"));
		}
	}
	CFRelease(byPid);
	return found;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

func sleepSystem() error {
	if r := C.sleepNow(); r != C.kIOReturnSuccess {
		return fmt.Errorf("IOPMSleepSystem: 0x%x", uint32(r))
	}
	return nil
}

func displaysAsleep() (bool, error) {
	switch C.screensAsleep() {
	case 1:
		return true, nil
	case 0:
		return false, nil
	}
	return false, errors.New("CGGetOnlineDisplayList failed")
}

func screenLocked() (bool, error) {
	switch C.sessionLocked() {
	case 1:
		return true, nil
	case 0:
		return false, nil
	}
	return false, errors.New("CGSessionCopyCurrentDictionary failed")
}

func declareUserActivity() error {
	if r := C.userActive(); r != C.kIOReturnSuccess {
		return fmt.Errorf("IOPMAssertionDeclareUserActivity: 0x%x", uint32(r))
	}
	return nil
}

func amphetamine(command string) (string, error) {
	out, err := exec.Command("/usr/bin/osascript", "-e", `tell application "Amphetamine" to `+command).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("amphetamine %q: %w: %s", command, err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func amphetamineAsserted() (bool, error) {
	switch C.amphAsserted() {
	case 1:
		return true, nil
	case 0:
		return false, nil
	}
	return false, errors.New("IOPMCopyAssertionsByProcess failed")
}

func amphetamineActive() (bool, error) {
	out, err := amphetamine("session is active")
	if err != nil {
		return false, err
	}
	switch out {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, fmt.Errorf("amphetamine session is active: %q", out)
}

// startAmphetamine is the right click on Amphetamine's menu bar icon: no options = Default Duration and Allow Display Sleep from its settings.
func startAmphetamine() error {
	_, err := amphetamine("start new session")
	return err
}

func endAmphetamine() error {
	_, err := amphetamine("end session")
	return err
}
