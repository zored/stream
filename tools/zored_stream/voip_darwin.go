package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework ApplicationServices -framework AppKit
#include <stdlib.h>
#include <ApplicationServices/ApplicationServices.h>
#include "ax_darwin.h"

static int voip_call_window(pid_t pid, int close) {
    AXUIElementRef app = AXUIElementCreateApplication(pid);
    CFArrayRef windows = NULL;
    int found = 0;
    if (AXUIElementCopyAttributeValue(app, kAXWindowsAttribute, (CFTypeRef *)&windows) == kAXErrorSuccess && windows) {
        for (CFIndex i = 0; i < CFArrayGetCount(windows) && !found; i++) {
            AXUIElementRef w = CFArrayGetValueAtIndex(windows, i);
            CFStringRef title = NULL;
            if (AXUIElementCopyAttributeValue(w, kAXTitleAttribute, (CFTypeRef *)&title) != kAXErrorSuccess || !title) continue;
            if (CFStringHasPrefix(title, CFSTR("In call"))) {
                found = 1;
                AXUIElementRef button = NULL;
                if (close && AXUIElementCopyAttributeValue(w, kAXCloseButtonAttribute, (CFTypeRef *)&button) == kAXErrorSuccess && button) {
                    AXUIElementPerformAction(button, kAXPressAction);
                    CFRelease(button);
                }
            }
            CFRelease(title);
        }
        CFRelease(windows);
    }
    CFRelease(app);
    return found;
}

static AXUIElementRef voip_main_window(AXUIElementRef app) {
    CFTypeRef w = NULL;
    if (AXUIElementCopyAttributeValue(app, kAXMainWindowAttribute, &w) != kAXErrorSuccess) return NULL;
    return w;
}

static void voip_make_main(AXUIElementRef w) {
    AXUIElementPerformAction(w, kAXRaiseAction);
    AXUIElementSetAttributeValue(w, kAXMainAttribute, kCFBooleanTrue);
    AXUIElementSetAttributeValue(w, kAXFocusedAttribute, kCFBooleanTrue);
}

static void voip_key_in_call(pid_t pid, int key) {
    AXUIElementRef app = AXUIElementCreateApplication(pid);
    AXUIElementRef prev = voip_main_window(app);
    AXUIElementRef call = NULL;
    CFArrayRef windows = NULL;
    if (AXUIElementCopyAttributeValue(app, kAXWindowsAttribute, (CFTypeRef *)&windows) == kAXErrorSuccess && windows) {
        for (CFIndex i = 0; i < CFArrayGetCount(windows) && !call; i++) {
            AXUIElementRef w = CFArrayGetValueAtIndex(windows, i);
            CFStringRef title = NULL;
            if (AXUIElementCopyAttributeValue(w, kAXTitleAttribute, (CFTypeRef *)&title) != kAXErrorSuccess || !title) continue;
            if (CFStringHasPrefix(title, CFSTR("In call"))) call = (AXUIElementRef)CFRetain(w);
            CFRelease(title);
        }
        CFRelease(windows);
    }
    int swap = call && prev && !CFEqual(call, prev);
    if (swap) {
        voip_make_main(call);
        usleep(100000);
    }
    ax_key(pid, key, AX_CMD | AX_SHIFT);
    if (swap) {
        usleep(150000);
        voip_make_main(prev);
    }
    if (call) CFRelease(call);
    if (prev) CFRelease(prev);
    CFRelease(app);
}

#import <AppKit/AppKit.h>

static pid_t voip_front_pid(void) {
    return NSWorkspace.sharedWorkspace.frontmostApplication.processIdentifier;
}

static void voip_activate(pid_t pid) {
    [[NSRunningApplication runningApplicationWithProcessIdentifier:pid] activateWithOptions:0];
}

*/
import "C"

import (
	"errors"
	"fmt"
	"time"

	"golang.org/x/sys/unix"
)

const (
	keyA     = 0
	keyV     = 9
	keyEnter = 76
)

func vkwsPid() (int, error) {
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return 0, fmt.Errorf("list processes: %w", err)
	}
	for _, p := range procs {
		if isVkwsProcess(unix.ByteSliceToString(p.Proc.P_comm[:])) {
			return int(p.Proc.P_pid), nil
		}
	}
	return 0, nil
}

func voipKey(pid, key int) {
	front := C.voip_front_pid()
	C.voip_activate(C.pid_t(pid))
	time.Sleep(100 * time.Millisecond)
	C.voip_key_in_call(C.pid_t(pid), C.int(key))
	if front > 0 && int(front) != pid {
		time.Sleep(150 * time.Millisecond)
		C.voip_activate(front)
	}
}

func voipCloseCallWindow(pid int) error {
	front := C.voip_front_pid()
	C.voip_activate(C.pid_t(pid))
	time.Sleep(100 * time.Millisecond)
	defer func() {
		if front > 0 && int(front) != pid {
			C.voip_activate(front)
		}
	}()
	if C.voip_call_window(C.pid_t(pid), 1) == 0 {
		return errors.New("no In call window")
	}
	for range 40 {
		time.Sleep(75 * time.Millisecond)
		if C.voip_call_window(C.pid_t(pid), 0) == 0 {
			return nil
		}
		C.ax_key(C.pid_t(pid), keyEnter, 0)
	}
	return errors.New("call window still open after Enter")
}
