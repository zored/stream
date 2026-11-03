package main

/*
#cgo LDFLAGS: -framework Carbon -framework Foundation
#include "input_darwin.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime"
)

var inputChanged func()

func init() {
	runtime.LockOSThread()
}

//export goInputChanged
func goInputChanged() {
	inputChanged()
}

func inputCurrent() (string, error) {
	var buf [64]C.char
	if C.tis_current(&buf[0], C.int(len(buf))) != 0 {
		return "", errors.New("no current keyboard input source")
	}
	return C.GoString(&buf[0]), nil
}

func inputNext() error {
	if status := C.tis_next(); status != 0 {
		return fmt.Errorf("TISSelectInputSource: %d", status)
	}
	return nil
}

// inputWatch runs the main thread run loop: TIS refreshes its cache and delivers notifications only there.
func inputWatch(changed func()) {
	inputChanged = changed
	C.tis_watch()
}
