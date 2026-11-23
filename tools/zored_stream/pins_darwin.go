package main

/*
#include <stdlib.h>
#include <CoreServices/CoreServices.h>

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
	"unsafe"
)

func launchURL(u string) error {
	s := C.CString(u)
	defer C.free(unsafe.Pointer(s))
	if status := C.open_url(s); status != 0 {
		return fmt.Errorf("open %s: LaunchServices error %d", u, status)
	}
	return nil
}
