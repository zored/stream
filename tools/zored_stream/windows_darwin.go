package main

/*
#cgo LDFLAGS: -framework ApplicationServices -framework AppKit
#include "ax_darwin.h"
#include "windows_darwin.h"
*/
import "C"

import "fmt"

const maxWindows = 256

func listWindows() ([]window, error) {
	var pids [maxWindows]C.int
	var ids [maxWindows]C.uint
	n := int(C.windows_list(&pids[0], &ids[0], maxWindows))
	if n < 0 {
		return nil, fmt.Errorf("CGWindowListCopyWindowInfo failed")
	}
	out := make([]window, n)
	for i := range out {
		out[i] = window{pid: int(pids[i]), id: uint32(ids[i])}
	}
	return out, nil
}

func raiseWindow(w window) error {
	if C.ax_trusted(0) == 0 {
		return fmt.Errorf("no accessibility permission")
	}
	if code := C.windows_raise(C.int(w.pid), C.uint(w.id)); code != 0 {
		return fmt.Errorf("raise window %d of pid %d: step %d failed", w.id, w.pid, code)
	}
	return nil
}
