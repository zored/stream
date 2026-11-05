package main

/*
#include <stdlib.h>
#include "calendar_darwin.h"
*/
import "C"

import (
	"cmp"
	"log"
	"unsafe"
)

const chatsScale = 2

type chatsDisc struct {
	avatar    []byte
	cx, cy, r float64
	rgb       uint32
	square    bool
}

type chatsLayer struct {
	discs []chatsDisc
	texts []calText
	alpha float64
}

func chatsPNG(w, h float64, discs []chatsDisc, texts []calText) string {
	return chatsLayersPNG(w, h, []chatsLayer{{discs, texts, 1}})
}

func chatsLayersPNG(w, h float64, layers []chatsLayer) string {
	c := C.cal_canvas_new(C.double(w), C.double(h), chatsScale, 0)
	for _, l := range layers {
		if l.alpha < 1 {
			C.cal_canvas_layer_begin(c, C.double(l.alpha))
		}
		for _, d := range l.discs {
			var p unsafe.Pointer
			if len(d.avatar) > 0 {
				p = C.CBytes(d.avatar)
			}
			if C.cal_canvas_avatar(c, p, C.int(len(d.avatar)), C.double(d.cx), C.double(d.cy), C.double(d.r), C.uint(cmp.Or(d.rgb, 0x333333)), cBool(d.square)) == 0 {
				log.Printf("chats: avatar decode failed (%d bytes)", len(d.avatar))
			}
			C.free(p)
		}
		for _, t := range l.texts {
			s, tail := C.CString(t.text), C.CString(t.tail)
			C.cal_canvas_text(c, s, tail, C.double(t.x), C.double(t.y), C.double(t.w), C.double(t.h), C.double(t.size), C.uint(t.rgb), C.uint(t.tailRGB), cBool(t.center))
			C.free(unsafe.Pointer(s))
			C.free(unsafe.Pointer(tail))
		}
		if l.alpha < 1 {
			C.cal_canvas_layer_end(c)
		}
	}
	uri, err := canvasEncode(c, true)
	if err != nil {
		log.Print(err)
	}
	return uri
}
