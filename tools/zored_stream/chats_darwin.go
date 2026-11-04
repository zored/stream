package main

/*
#include <stdlib.h>
#include "calendar_darwin.h"
*/
import "C"

import (
	"log"
	"unsafe"
)

type chatsDisc struct {
	avatar    []byte
	cx, cy, r float64
}

func chatsPNG(w, h float64, discs []chatsDisc, texts []calText) string {
	c := C.cal_canvas_new(C.double(w), C.double(h), 4, 0)
	for _, d := range discs {
		var p unsafe.Pointer
		if len(d.avatar) > 0 {
			p = C.CBytes(d.avatar)
		}
		if C.cal_canvas_avatar(c, p, C.int(len(d.avatar)), C.double(d.cx), C.double(d.cy), C.double(d.r), C.uint(0x333333)) == 0 {
			log.Printf("chats: avatar decode failed (%d bytes)", len(d.avatar))
		}
		C.free(p)
	}
	for _, t := range texts {
		s, tail := C.CString(t.text), C.CString(t.tail)
		C.cal_canvas_text(c, s, tail, C.double(t.x), C.double(t.y), C.double(t.w), C.double(t.h), C.double(t.size), C.uint(t.rgb), C.uint(t.tailRGB), cBool(t.center))
		C.free(unsafe.Pointer(s))
		C.free(unsafe.Pointer(tail))
	}
	uri, err := canvasPNG(c)
	if err != nil {
		log.Print(err)
	}
	return uri
}
