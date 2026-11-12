package main

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework AppKit -framework ApplicationServices -framework ImageIO
#include <stdlib.h>
#include "ax_darwin.h"
#include "calendar_darwin.h"
#include "switcher_darwin.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unsafe"
)

func switchApps() ([]switchApp, error) {
	out := C.sw_apps(C.int(C.ax_front_pid()))
	defer C.free(unsafe.Pointer(out))
	var apps []switchApp
	for line := range strings.Lines(C.GoString(out)) {
		pid, name, _ := strings.Cut(strings.TrimSuffix(line, "\n"), "\t")
		n, err := strconv.Atoi(pid)
		if err != nil {
			return nil, fmt.Errorf("switcher: app line %q: %w", line, err)
		}
		apps = append(apps, switchApp{pid: n, name: name})
	}
	return apps, nil
}

func switchWindows(pid int) ([]string, error) {
	if C.ax_trusted(0) == 0 {
		return nil, errors.New("no Accessibility permission")
	}
	out := C.sw_windows(C.int(pid))
	if out == nil {
		return nil, errors.New("AXWindows failed")
	}
	defer C.free(unsafe.Pointer(out))
	var titles []string
	for line := range strings.Lines(C.GoString(out)) {
		titles = append(titles, strings.TrimSuffix(line, "\n"))
	}
	return titles, nil
}

func switchRaise(pid, window int) error {
	if code := C.sw_raise(C.int(pid), C.int(window)); code != 0 {
		return fmt.Errorf("sw_raise: %d", code)
	}
	return nil
}

var switchIcons sync.Map

func switchIcon(pid string, turn float64) ([]byte, error) {
	key := fmt.Sprintf("%s %g", pid, turn)
	if icon, ok := switchIcons.Load(key); ok {
		return icon.([]byte), nil
	}
	n, err := strconv.Atoi(pid)
	if err != nil {
		return nil, err
	}
	var size C.int
	p := C.sw_icon(C.int(n), C.double(turn), switchIconSize*4, &size)
	if p == nil {
		return nil, fmt.Errorf("switcher: no icon for pid %s", pid)
	}
	defer C.free(p)
	icon := C.GoBytes(p, size)
	switchIcons.Store(key, icon)
	return icon, nil
}

func switchPNG(side int, glyphs []emojiGlyph, ringY float64) (string, error) {
	c := C.cal_canvas_new(stripDrawW, stripDrawH, 1.25, 0)
	var failed error
	for _, g := range glyphs {
		if side == emojiRight {
			C.cal_canvas_ring(c, stripDrawW/2, C.double(g.y), switchDot/2.0, switchDot, 0xffffff)
			continue
		}
		icon, err := switchIcon(g.text, g.turn)
		if err != nil {
			failed = err
			continue
		}
		C.cal_canvas_avatar(c, unsafe.Pointer(&icon[0]), C.int(len(icon)), stripDrawW/2, C.double(g.y), switchIconSize/2, 0, 1)
	}
	C.cal_canvas_ring(c, stripDrawW/2, C.double(ringY), emojiRing, 2, grayRGB)
	uri, err := canvasPNG(c)
	return uri, errors.Join(err, failed)
}

func switchTitlePNG(text string) (string, error) {
	c := C.cal_canvas_new(100, 100, 1, 0)
	s, tail := C.CString(text), C.CString("")
	defer C.free(unsafe.Pointer(s))
	defer C.free(unsafe.Pointer(tail))
	C.cal_canvas_text(c, s, tail, 6, 6, 88, 88, 15, 0xffffff, 0xffffff, 0)
	return canvasPNG(c)
}
