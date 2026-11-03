package main

/*
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation
#include <stdlib.h>
#include <CoreGraphics/CoreGraphics.h>
#include "calendar_darwin.h"

static int unicode_post(const UniChar *s, int n) {
    for (int down = 1; down >= 0; down--) {
        CGEventRef e = CGEventCreateKeyboardEvent(NULL, 0, down);
        if (!e) return 0;
        CGEventKeyboardSetUnicodeString(e, n, s);
        CGEventPost(kCGHIDEventTap, e);
        CFRelease(e);
    }
    return 1;
}
*/
import "C"

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"unicode/utf16"
	"unsafe"
)

type emojiGlyph struct {
	text    string
	y, size float64
	alpha   float64
}

func emojiPNG(glyphs []emojiGlyph, ring bool) (string, error) {
	c := C.cal_canvas_new(stripDrawW, stripDrawH, 1.25, 0)
	if ring {
		C.cal_canvas_ring(c, stripDrawW/2, emojiTop, emojiRing, 2, grayRGB)
	}
	for _, g := range glyphs {
		s := C.CString(g.text)
		C.cal_canvas_glyph(c, s, stripDrawW/2, C.double(g.y), C.double(g.size), C.double(g.alpha))
		C.free(unsafe.Pointer(s))
	}
	return canvasPNG(c)
}

func emojiTilePNG(text string, size float64) (string, error) {
	c := C.cal_canvas_new(100, 100, 1, 0)
	s := C.CString(text)
	C.cal_canvas_glyph(c, s, 50, 50, C.double(size), 1)
	C.free(unsafe.Pointer(s))
	return canvasPNG(c)
}

func emojiType(s string) error {
	if !C.CGPreflightPostEventAccess() {
		return errors.New("no permission to post events")
	}
	u := utf16.Encode([]rune(s))
	if len(u) == 0 {
		return errors.New("empty emoji")
	}
	if C.unicode_post((*C.UniChar)(unsafe.Pointer(&u[0])), C.int(len(u))) == 0 {
		return errors.New("CGEventCreateKeyboardEvent failed")
	}
	return nil
}

func emojiRecents() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	plist := filepath.Join(home, "Library/Preferences/com.apple.EmojiPreferences.plist")
	out, err := exec.Command("plutil", "-extract", "EMFDefaultsKey.EMFRecentsKey", "json", "-o", "-", plist).Output()
	if err != nil {
		return nil, fmt.Errorf("emoji recents: %w", err)
	}
	var recents []string
	if err := json.Unmarshal(out, &recents); err != nil {
		return nil, fmt.Errorf("emoji recents json: %w", err)
	}
	return recents, nil
}
