package main

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework EventKit -framework Foundation -framework CoreText -framework CoreGraphics -framework ImageIO
#include <stdlib.h>
#include "calendar_darwin.h"
*/
import "C"

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"time"
	"unsafe"
)

type calRaw struct {
	ID       string  `json:"id"`
	Ext      string  `json:"ext"`
	Title    string  `json:"title"`
	Start    float64 `json:"start"`
	End      float64 `json:"end"`
	AllDay   bool    `json:"allDay"`
	Reminder bool    `json:"reminder"`
	Done     bool    `json:"done"`
}

func calFetch(from, to time.Time) ([]calItem, error) {
	var cerr *C.char
	out := C.cal_items(C.double(from.Unix()), C.double(to.Unix()), &cerr)
	if out == nil {
		defer C.free(unsafe.Pointer(cerr))
		return nil, errors.New(C.GoString(cerr))
	}
	defer C.free(unsafe.Pointer(out))
	var raw []calRaw
	if err := json.Unmarshal([]byte(C.GoString(out)), &raw); err != nil {
		return nil, fmt.Errorf("calendar json: %w", err)
	}
	items := make([]calItem, len(raw))
	for i, r := range raw {
		items[i] = calItem{id: r.ID, ext: r.Ext, title: r.Title, start: unixFloat(r.Start), end: unixFloat(r.End),
			allDay: r.AllDay, reminder: r.Reminder, done: r.Done}
	}
	return items, nil
}

func unixFloat(f float64) time.Time {
	return time.Unix(0, int64(f*1e9))
}

func calSetDone(id string, done bool) error {
	c := C.CString(id)
	defer C.free(unsafe.Pointer(c))
	if e := C.cal_set_done(c, cBool(done)); e != nil {
		defer C.free(unsafe.Pointer(e))
		return errors.New(C.GoString(e))
	}
	return nil
}

type calText struct {
	text       string
	x, y, w, h float64
	size       float64
	rgb        uint32
	center     bool
}

func calPNG(w, h float64, texts []calText) (string, error) {
	c := C.cal_canvas_new(C.double(w), C.double(h))
	for _, t := range texts {
		s := C.CString(t.text)
		C.cal_canvas_text(c, s, C.double(t.x), C.double(t.y), C.double(t.w), C.double(t.h), C.double(t.size), C.uint(t.rgb), cBool(t.center))
		C.free(unsafe.Pointer(s))
	}
	var n C.int
	p := C.cal_canvas_png(c, &n)
	if p == nil {
		return "", errors.New("calendar png: encode failed")
	}
	defer C.free(p)
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(C.GoBytes(p, n)), nil
}

func calOpen(it calItem) error {
	u := "ical://ekevent/" + url.PathEscape(it.ext) + "?method=show&options=more"
	if out, err := exec.Command("open", u).CombinedOutput(); err != nil {
		return fmt.Errorf("open %s: %v: %s", u, err, out)
	}
	return nil
}
