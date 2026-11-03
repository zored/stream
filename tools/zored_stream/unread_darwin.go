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

func unreadScan() (badges map[string]string, banners [][2]string) {
	s := C.unread_scan()
	defer C.free(unsafe.Pointer(s))
	badges = map[string]string{}
	for line := range strings.SplitSeq(C.GoString(s), "\n") {
		f := strings.SplitN(line, "\t", 3)
		if len(f) != 3 {
			continue
		}
		if f[0] == "badge" {
			badges[f[1]] = f[2]
		} else {
			banners = append(banners, [2]string{f[1], f[2]})
		}
	}
	return badges, banners
}

func unreadOpen(bundle string) error {
	return exec.Command("open", "-b", bundle).Run()
}
