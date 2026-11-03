package main

/*
#cgo LDFLAGS: -framework AppKit -framework ApplicationServices
#include "mediakey_darwin.h"
#include "ax_darwin.h"
*/
import "C"

import "errors"

func postMediaKey(key int) error {
	if C.ax_trusted(0) == 0 {
		return errors.New("zored_stream needs Accessibility")
	}
	if C.mediakey_post(C.int(key)) == 0 {
		return errors.New("can't create media key event")
	}
	return nil
}
