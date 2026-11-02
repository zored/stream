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
*/
import "C"

import "errors"

func postScroll(vertical, horizontal int) error {
	if !C.CGPreflightPostEventAccess() {
		return errors.New("no permission to post events")
	}
	if C.scroll_post(C.int(vertical), C.int(horizontal)) == 0 {
		return errors.New("CGEventCreateScrollWheelEvent failed")
	}
	return nil
}
