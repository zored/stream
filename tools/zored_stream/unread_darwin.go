package main

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework AppKit -framework ApplicationServices
#include <stdlib.h>
#include "unread_darwin.h"
*/
import "C"

import (
	"os/exec"
	"strings"
	"unsafe"
)

func unreadBadges() map[string]string {
	s := C.unread_badges()
	defer C.free(unsafe.Pointer(s))
	badges := map[string]string{}
	for line := range strings.SplitSeq(C.GoString(s), "\n") {
		if name, badge, ok := strings.Cut(line, "\t"); ok {
			badges[name] = badge
		}
	}
	return badges
}

func unreadOpen(bundle string) error {
	return exec.Command("open", "-b", bundle).Run()
}
