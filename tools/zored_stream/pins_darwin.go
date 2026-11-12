package main

/*
#include <stdlib.h>
#include <CoreServices/CoreServices.h>
#include "ax_darwin.h"

static OSStatus open_url(const char *s) {
    CFURLRef url = CFURLCreateWithBytes(NULL, (const UInt8 *)s, strlen(s), kCFStringEncodingUTF8, NULL);
    if (!url) return kLSDataErr;
    OSStatus status = LSOpenCFURLRef(url, NULL);
    CFRelease(url);
    return status;
}
*/
import "C"

import (
	"fmt"
	"time"
	"unsafe"
)

const (
	sidebarMenuWait = time.Second
	sidebarMenuPoll = 20 * time.Millisecond
)

func launchURL(u string) error {
	s := C.CString(u)
	defer C.free(unsafe.Pointer(s))
	if status := C.open_url(s); status != 0 {
		return fmt.Errorf("open %s: LaunchServices error %d", u, status)
	}
	return nil
}

func sidebarPinChat(title string, on bool) error {
	pid := claudePid()
	if pid == 0 {
		return fmt.Errorf("Claude is not running")
	}
	want, other := "Unpin", "Pin"
	if on {
		want, other = other, want
	}
	return inClaudeWindow(pid, func(w C.AXUIElementRef) error {
		more := findAttr(w, "AXPopUpButton", "AXDescription", "More options for "+title, false)
		if more == 0 {
			return fmt.Errorf("no sidebar row")
		}
		defer C.ax_release(more)
		if code := C.ax_press(more); code != 0 {
			return fmt.Errorf("press More options: AX error %d", code)
		}
		for deadline := time.Now().Add(sidebarMenuWait); time.Now().Before(deadline); time.Sleep(sidebarMenuPoll) {
			if item := findAttr(w, "AXMenuItem", "AXTitle", want, false); item != 0 {
				defer C.ax_release(item)
				if code := C.ax_press(item); code != 0 {
					return fmt.Errorf("press %s: AX error %d", want, code)
				}
				return nil
			}
			if item := findAttr(w, "AXMenuItem", "AXTitle", other, false); item != 0 {
				C.ax_release(item)
				C.ax_key(pid, keyEscape, 0)
				return nil
			}
		}
		C.ax_key(pid, keyEscape, 0)
		return fmt.Errorf("no %s in the menu after %s", want, sidebarMenuWait)
	})
}
