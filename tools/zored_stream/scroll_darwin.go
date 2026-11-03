package main

/*
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation
#include <CoreGraphics/CoreGraphics.h>

static int scroll_post(int vertical, int horizontal) {
    CGEventRef e = CGEventCreateScrollWheelEvent(NULL, kCGScrollEventUnitLine, 2, vertical, horizontal);
    if (!e) return 0;
    CGEventPost(kCGHIDEventTap, e);
    CFRelease(e);
    return 1;
}

static int key_post(CGKeyCode key, bool down, CGEventFlags flags) {
    CGEventRef e = CGEventCreateKeyboardEvent(NULL, key, down);
    if (!e) return 0;
    CGEventSetFlags(e, flags);
    CGEventPost(kCGHIDEventTap, e);
    CFRelease(e);
    return 1;
}
*/
import "C"

import "errors"

const (
	keyTab     = 48
	keyGrave   = 50
	keyCommand = 55
	keyControl = 59
)

func postScroll(vertical, horizontal int) error {
	if !C.CGPreflightPostEventAccess() {
		return errors.New("no permission to post events")
	}
	if C.scroll_post(C.int(vertical), C.int(horizontal)) == 0 {
		return errors.New("CGEventCreateScrollWheelEvent failed")
	}
	return nil
}

func postKey(key C.CGKeyCode, down bool, flags C.CGEventFlags) error {
	if !C.CGPreflightPostEventAccess() {
		return errors.New("no permission to post events")
	}
	if C.key_post(key, C.bool(down), flags) == 0 {
		return errors.New("CGEventCreateKeyboardEvent failed")
	}
	return nil
}

func modifierKey(m modifier) (C.CGKeyCode, C.CGEventFlags) {
	if m == modCmd {
		return keyCommand, C.kCGEventFlagMaskCommand
	}
	return keyControl, C.kCGEventFlagMaskControl
}

func postModifier(m modifier, down bool) error {
	key, flags := modifierKey(m)
	if !down {
		flags = 0
	}
	return postKey(key, down, flags)
}

func postChord(m modifier, c chord, shift bool) error {
	_, flags := modifierKey(m)
	key := C.CGKeyCode(keyTab)
	if c == chordGrave {
		key = keyGrave
	}
	if shift {
		flags |= C.kCGEventFlagMaskShift
	}
	if err := postKey(key, true, flags); err != nil {
		return err
	}
	return postKey(key, false, flags)
}
