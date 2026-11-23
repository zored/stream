package main

/*
#cgo LDFLAGS: -framework ApplicationServices -framework AppKit
#include <stdlib.h>
#include "ax_darwin.h"
#include "calendar_darwin.h"

void *orca_svg_png(const void *svg, int len, int px, int *out_len);

static CFStringRef orca_str(AXUIElementRef e, CFStringRef attr) {
    CFTypeRef v = NULL;
    if (AXUIElementCopyAttributeValue(e, attr, &v) != kAXErrorSuccess || !v) return NULL;
    if (CFGetTypeID(v) != CFStringGetTypeID()) { CFRelease(v); return NULL; }
    return v;
}

// orca_buttons lists the titles and descriptions of AXButtons under root, for a log line when a tab is missing.
static char *orca_buttons(AXUIElementRef root, int max_nodes, int *nodes, int *windows) {
    CFTypeRef ws = NULL;
    *windows = -1;
    if (AXUIElementCopyAttributeValue(root, kAXWindowsAttribute, &ws) == kAXErrorSuccess && ws) {
        *windows = CFGetTypeID(ws) == CFArrayGetTypeID() ? (int)CFArrayGetCount(ws) : -2;
        CFRelease(ws);
    }
    CFMutableArrayRef queue = CFArrayCreateMutable(NULL, 0, &kCFTypeArrayCallBacks);
    CFMutableSetRef seen = CFSetCreateMutable(NULL, 0, &kCFTypeSetCallBacks);
    CFMutableStringRef out = CFStringCreateMutable(NULL, 0);
    CFArrayAppendValue(queue, root);
    CFSetAddValue(seen, root);
    CFIndex next = 0;
    for (; next < CFArrayGetCount(queue) && next < max_nodes; next++) {
        AXUIElementRef e = (AXUIElementRef)CFArrayGetValueAtIndex(queue, next);
        CFStringRef role = orca_str(e, kAXRoleAttribute);
        if (role && CFEqual(role, kAXButtonRole) && CFStringGetLength(out) < 4000) {
            CFStringRef t = orca_str(e, kAXTitleAttribute), d = orca_str(e, kAXDescriptionAttribute);
            CFStringAppendFormat(out, NULL, CFSTR("[%@ | %@] "), t ? t : CFSTR(""), d ? d : CFSTR(""));
            if (t) CFRelease(t);
            if (d) CFRelease(d);
        }
        if (role) CFRelease(role);
        CFTypeRef children = NULL;
        if (AXUIElementCopyAttributeValue(e, kAXChildrenAttribute, &children) != kAXErrorSuccess || !children) continue;
        if (CFGetTypeID(children) == CFArrayGetTypeID()) {
            for (CFIndex i = 0; i < CFArrayGetCount(children); i++) {
                CFTypeRef c = CFArrayGetValueAtIndex(children, i);
                if (CFSetContainsValue(seen, c)) continue;
                CFSetAddValue(seen, c);
                CFArrayAppendValue(queue, c);
            }
        }
        CFRelease(children);
    }
    *nodes = (int)next;
    CFIndex size = CFStringGetMaximumSizeForEncoding(CFStringGetLength(out), kCFStringEncodingUTF8) + 1;
    char *buf = malloc(size);
    if (!CFStringGetCString(out, buf, size, kCFStringEncodingUTF8)) buf[0] = 0;
    CFRelease(out);
    CFRelease(queue);
    CFRelease(seen);
    return buf;
}
*/
import "C"

import (
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"cmp"
	"log"
	"math"
	"time"
	"unsafe"
)

const (
	orcaBundle = "com.stablyai.orca"
	orcaApp    = "file:///Applications/Orca.app"
)

func orcaPid() (C.pid_t, error) {
	cb := C.CString(orcaBundle)
	defer C.free(unsafe.Pointer(cb))
	pid := C.ax_app_pid(cb)
	if pid == 0 {
		return 0, fmt.Errorf("orca is not running")
	}
	return pid, nil
}

func orcaFront() error {
	return launchURL(orcaApp)
}

// orcaPressTab presses the chat tab of the active workspace: an AXButton titled `[<status> ]<name> Close tab <name>`; the close button carries the same text only in AXDescription.
func orcaPressTab(name string) error {
	pid, err := orcaPid()
	if err != nil {
		return err
	}
	app := C.ax_app(pid)
	defer C.ax_release(app)
	cRole, cAttr, cText := C.CString("AXButton"), C.CString("AXTitle"), C.CString(name+" Close tab "+name)
	defer C.free(unsafe.Pointer(cRole))
	defer C.free(unsafe.Pointer(cAttr))
	defer C.free(unsafe.Pointer(cText))
	deadline := time.Now().Add(orcaTabWait)
	for {
		var truncated C.int
		tab := C.ax_find_attr(app, cRole, cAttr, cText, C.AX_MATCH_SUFFIX, axMaxNodes, &truncated)
		if tab != 0 {
			code := C.ax_press(tab)
			C.ax_release(tab)
			if code != 0 {
				return fmt.Errorf("AXPress on tab %q: AXError %d", name, int(code))
			}
			return nil
		}
		if time.Now().After(deadline) {
			hint := ""
			if truncated != 0 {
				hint = fmt.Sprintf(", AX walk stopped at %d nodes", axMaxNodes)
			}
			var nodes, windows C.int
			buttons := C.orca_buttons(app, axMaxNodes, &nodes, &windows)
			defer C.free(unsafe.Pointer(buttons))
			return fmt.Errorf("no tab %q in Orca after %s%s; %d windows, %d nodes, buttons: %s",
				name, orcaTabWait, hint, int(windows), int(nodes), C.GoString(buttons))
		}
		time.Sleep(orcaTabEvery)
	}
}

const orcaIconPx = 96

var orcaIcons sync.Map

// orcaIconPNG is the chat badge: the emoji its title starts with, else the logo of its model; nil when neither.
func orcaIconPNG(c orcaChat) []byte {
	key := cmp.Or(titleEmoji(c.name), c.model)
	if key == "" {
		return nil
	}
	if b, ok := orcaIcons.Load(key); ok {
		return b.([]byte)
	}
	var b []byte
	var err error
	if e := titleEmoji(c.name); e != "" {
		b, err = orcaEmojiPNG(e)
	} else {
		b, err = orcaLogoPNG(c.model)
	}
	if err != nil {
		log.Printf("orca: icon %q: %v", key, err)
		return nil
	}
	orcaIcons.Store(key, b)
	return b
}

func orcaEmojiPNG(e string) ([]byte, error) {
	const s = 40.0
	uri, err := calPNG(s, s, false, 0, []calText{{text: e, x: 0, y: s * 0.2, w: s, h: s * 0.75, size: s * 0.5, rgb: whiteRGB, center: true}})
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(uri[strings.Index(uri, ",")+1:])
}

func orcaLogoPNG(icon string) ([]byte, error) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="96" height="96" viewBox="0 0 100 100">` + iconAt(icon, "#fff", 50, 50, 56) + `</svg>`
	p := C.CBytes([]byte(svg))
	defer C.free(p)
	var n C.int
	out := C.orca_svg_png(p, C.int(len(svg)), orcaIconPx, &n)
	if out == nil {
		return nil, fmt.Errorf("svg render failed")
	}
	defer C.free(out)
	return C.GoBytes(out, n), nil
}

// orcaStripPNG draws the chat strip; bar > 0 grows along the bottom edge (forward push), < 0 along the top (back), as in chats.
func orcaStripPNG(w, h, bar float64, texts ...calText) (string, error) {
	c := C.cal_canvas_new(C.double(w), C.double(h), 4, 1)
	for _, t := range texts {
		s, tail := C.CString(t.text), C.CString(t.tail)
		C.cal_canvas_text(c, s, tail, C.double(t.x), C.double(t.y), C.double(t.w), C.double(t.h), C.double(t.size), C.uint(t.rgb), C.uint(t.tailRGB), cBool(t.center))
		C.free(unsafe.Pointer(s))
		C.free(unsafe.Pointer(tail))
	}
	if bw := (w - 8) * min(math.Abs(bar), 1); bw > 0 {
		y, x := h-chatsBarH, 4.0
		if bar < 0 {
			y, x = 0, w-4-bw
		}
		C.cal_canvas_rect(c, C.double(x), C.double(y), C.double(bw), chatsBarH, whiteRGB)
	}
	return canvasPNG(c)
}
