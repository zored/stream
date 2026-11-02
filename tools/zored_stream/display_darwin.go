package main

/*
#cgo LDFLAGS: -F/System/Library/PrivateFrameworks -framework DisplayServices -framework CoreGraphics
#include <CoreGraphics/CoreGraphics.h>
extern int DisplayServicesGetBrightness(CGDirectDisplayID display, float *brightness);
extern int DisplayServicesSetBrightness(CGDirectDisplayID display, float brightness);
*/
import "C"

import (
	"fmt"
	"math"
)

func readDisplayBrightness() (int, error) {
	var b C.float
	if status := C.DisplayServicesGetBrightness(C.CGMainDisplayID(), &b); status != 0 {
		return 0, fmt.Errorf("DisplayServicesGetBrightness: %d", int(status))
	}
	return int(math.Round(float64(b) * 100)), nil
}

func writeDisplayBrightness(value int) error {
	if status := C.DisplayServicesSetBrightness(C.CGMainDisplayID(), C.float(float64(value)/100)); status != 0 {
		return fmt.Errorf("DisplayServicesSetBrightness: %d", int(status))
	}
	return nil
}
