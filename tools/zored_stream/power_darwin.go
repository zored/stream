package main

/*
#cgo LDFLAGS: -framework IOKit -framework CoreFoundation
#include <IOKit/pwr_mgt/IOPMLib.h>

static IOReturn sleepNow(void) {
	io_connect_t port = IOPMFindPowerManagement(MACH_PORT_NULL);
	if (port == MACH_PORT_NULL) {
		return kIOReturnNotFound;
	}
	IOReturn r = IOPMSleepSystem(port);
	IOServiceClose(port);
	return r;
}

static IOReturn userActive(void) {
	IOPMAssertionID id;
	return IOPMAssertionDeclareUserActivity(CFSTR("zored_stream keep-awake off"), kIOPMUserActiveLocal, &id);
}
*/
import "C"

import (
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
