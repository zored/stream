package main

/*
#include <stdlib.h>
#include "ax_darwin.h"
*/
import "C"

import (
	"fmt"
	"os/exec"
	"strings"
	"unsafe"
)

func itermPid() C.pid_t {
	bundle := C.CString(itermBundle)
	defer C.free(unsafe.Pointer(bundle))
	return C.ax_app_pid(bundle)
}

func itermCurrent() (string, error) {
	out, err := exec.Command("/usr/bin/osascript", "-e",
		`tell application id "`+itermBundle+`" to get unique id of current session of current window`).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("iTerm2 current session: %v: %s", err, out)
	}
	return strings.TrimSpace(string(out)), nil
}
