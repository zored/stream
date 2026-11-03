package main

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework EventKit -framework Foundation
#include <stdlib.h>
#include "calendar_darwin.h"
*/
import "C"

import (
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
			allDay: r.AllDay, reminder: r.Reminder}
	}
	return items, nil
}

func unixFloat(f float64) time.Time {
	return time.Unix(0, int64(f*1e9))
}

func calComplete(id string) error {
	c := C.CString(id)
	defer C.free(unsafe.Pointer(c))
	if e := C.cal_complete(c); e != nil {
		defer C.free(unsafe.Pointer(e))
		return errors.New(C.GoString(e))
	}
	return nil
}

func calOpen(it calItem) error {
	u := "ical://ekevent/" + url.PathEscape(it.ext) + "?method=show&options=more"
	if out, err := exec.Command("open", u).CombinedOutput(); err != nil {
		return fmt.Errorf("open %s: %v: %s", u, err, out)
	}
	return nil
}
