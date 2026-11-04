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

static IOReturn holdDisplay(IOPMAssertionID *id) {
	return IOPMAssertionCreateWithName(kIOPMAssertPreventUserIdleDisplaySleep, kIOPMAssertionLevelOn, CFSTR("zored_stream keep-awake restart"), id);
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
	"strconv"
	"strings"
	"time"
)

func sleepSystem() error {
	if r := C.sleepNow(); r != C.kIOReturnSuccess {
		return fmt.Errorf("IOPMSleepSystem: 0x%x", uint32(r))
	}
	return nil
}

func declareUserActivity() error {
	if r := C.userActive(); r != C.kIOReturnSuccess {
		return fmt.Errorf("IOPMAssertionDeclareUserActivity: 0x%x", uint32(r))
	}
	return nil
}

func bridgeDisplay(d time.Duration) error {
	var id C.IOPMAssertionID
	if r := C.holdDisplay(&id); r != C.kIOReturnSuccess {
		return fmt.Errorf("IOPMAssertionCreateWithName: 0x%x", uint32(r))
	}
	time.AfterFunc(d, func() { C.IOPMAssertionRelease(id) })
	return nil
}

func amphetamine(command string) (string, error) {
	out, err := exec.Command("/usr/bin/osascript", "-e", `tell application "Amphetamine" to `+command).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("amphetamine %q: %w: %s", command, err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

type amphSession struct {
	active bool
	left   time.Duration
	timed  bool
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

func readAmphetamine() (amphSession, error) {
	out, err := amphetamine("return {session is active, session time remaining}")
	if err != nil {
		return amphSession{}, err
	}
	active, left, ok := strings.Cut(out, ", ")
	seconds, err := strconv.Atoi(left)
	if !ok || err != nil {
		return amphSession{}, fmt.Errorf("amphetamine session: %q", out)
	}
	return amphSession{active: active == "true", left: time.Duration(seconds) * time.Second, timed: seconds > 0}, nil
}

func startAmphetamine(d time.Duration) error {
	_, err := amphetamine(fmt.Sprintf("start new session with options {duration:%d, interval:minutes, displaySleepAllowed:false}", int(d/time.Minute)))
	return err
}

func endAmphetamine() error {
	_, err := amphetamine("end session")
	return err
}
