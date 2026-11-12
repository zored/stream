package main

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -F/System/Library/PrivateFrameworks -framework SkyLight -framework AppKit -framework ApplicationServices -framework ImageIO
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

func switchFrontPid() int {
	return int(C.ax_front_pid())
}

func switchIcon(pid string, turn float64, px int) ([]byte, error) {
	key := fmt.Sprintf("%s %g %d", pid, turn, px)
	if icon, ok := switchIcons.Load(key); ok {
		return icon.([]byte), nil
	}
	n, err := strconv.Atoi(pid)
	if err != nil {
		return nil, err
	}
	var size C.int
	p := C.sw_icon(C.int(n), C.double(turn), C.int(px), &size)
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
		icon, err := switchIcon(g.text, g.turn, switchIconSize*4)
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

func switchCardPNG(text string) (string, error) {
	const w, h = stripDrawH, stripDrawW
	return calPNG(w, h, true, 0, []calText{{text: text, x: 4, y: 3, w: w - 8, h: 44, size: 15, rgb: whiteRGB}})
}

func switchAvatar(c unsafe.Pointer, pid int, cx, cy, r float64) error {
	icon, err := switchIcon(strconv.Itoa(pid), 0, int(r*8))
	if err != nil {
		return err
	}
	C.cal_canvas_avatar(c, unsafe.Pointer(&icon[0]), C.int(len(icon)), C.double(cx), C.double(cy), C.double(r), 0, 1)
	return nil
}

func switchBigPNG(pid int) (string, error) {
	c := C.cal_canvas_new(100, 100, 2, 0)
	failed := switchAvatar(c, pid, 50, 50, 44)
	uri, err := canvasPNG(c)
	return uri, errors.Join(err, failed)
}

func switchRecentPNG(apps []switchApp, badges []bool) (string, error) {
	const r, dot = 20, 4
	c := C.cal_canvas_new(100, 100, 2, 0)
	var failed []error
	for i, a := range apps {
		cx, cy := 28+44*float64(i%2), 28+44*float64(i/2)
		failed = append(failed, switchAvatar(c, a.pid, cx, cy, r))
		if badges[i] {
			C.cal_canvas_avatar(c, nil, 0, C.double(cx+r-dot), C.double(cy-r+dot), dot+1.5, 0, 0)
			C.cal_canvas_avatar(c, nil, 0, C.double(cx+r-dot), C.double(cy-r+dot), dot, whiteRGB, 0)
		}
	}
	uri, err := canvasPNG(c)
	return uri, errors.Join(append(failed, err)...)
}
