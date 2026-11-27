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
static int orca_action(AXUIElementRef e, const char *name) {
    CFStringRef a = CFStringCreateWithCString(NULL, name, kCFStringEncodingUTF8);
    int code = AXUIElementPerformAction(e, a);
    CFRelease(a);
    return code;
}

// orca_cancel_menu closes the context menu holding item.
static int orca_cancel_menu(AXUIElementRef item) {
    CFTypeRef menu = NULL;
    if (AXUIElementCopyAttributeValue(item, kAXParentAttribute, &menu) != kAXErrorSuccess || !menu) return -1;
    int code = AXUIElementPerformAction(menu, kAXCancelAction);
    CFRelease(menu);
    return code;
}
*/
import "C"

import (
	"cmp"
	"encoding/base64"
	"fmt"
	"log"
	"math"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unsafe"
)

const (
	orcaBundle = "com.stablyai.orca.dev"
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
	if out, err := exec.Command("open", "-b", orcaBundle).CombinedOutput(); err != nil {
		return fmt.Errorf("open -b %s: %w %s", orcaBundle, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// orcaRow finds the chat's row in the sidebar outline: an AXRow titled `[<status>[ · Unread] ]<name> - <preview> <age>`, there for every workspace and host; waits up to orcaTabWait. The caller releases it.
func orcaRow(app C.AXUIElementRef, name string) (C.AXUIElementRef, error) {
	cRole, cAttr := C.CString("AXRow"), C.CString("AXTitle")
	defer C.free(unsafe.Pointer(cRole))
	defer C.free(unsafe.Pointer(cAttr))
	queries := []struct {
		text  string
		match C.int
	}{{" " + name + " - ", C.AX_MATCH_CONTAINS}, {name + " - ", C.AX_MATCH_PREFIX}}
	deadline := time.Now().Add(orcaTabWait)
	for {
		var truncated C.int
		for _, q := range queries {
			cText := C.CString(q.text)
			row := C.ax_find_attr(app, cRole, cAttr, cText, q.match, axMaxNodes, &truncated)
			C.free(unsafe.Pointer(cText))
			if row != 0 {
				return row, nil
			}
		}
		if time.Now().After(deadline) {
			hint := ""
			if truncated != 0 {
				hint = fmt.Sprintf(", AX walk stopped at %d nodes", axMaxNodes)
			}
			var nodes, windows C.int
			buttons := C.orca_buttons(app, axMaxNodes, &nodes, &windows)
			defer C.free(unsafe.Pointer(buttons))
			return 0, fmt.Errorf("no sidebar row %q in Orca after %s%s; %d windows, %d nodes, buttons: %s",
				name, orcaTabWait, hint, int(windows), int(nodes), C.GoString(buttons))
		}
		time.Sleep(orcaTabEvery)
	}
}

func orcaAXApp() (C.AXUIElementRef, error) {
	pid, err := orcaPid()
	if err != nil {
		return 0, err
	}
	return C.ax_app(pid), nil
}

// orcaPressRow presses the chat's sidebar row: that switches workspace and tab in one go.
func orcaPressRow(name string) error {
	app, err := orcaAXApp()
	if err != nil {
		return err
	}
	defer C.ax_release(app)
	row, err := orcaRow(app, name)
	if err != nil {
		return err
	}
	defer C.ax_release(row)
	if code := C.ax_press(row); code != 0 {
		return fmt.Errorf("AXPress on row %q: AXError %d", name, int(code))
	}
	return nil
}

// orcaCloseTab switches to the chat by its sidebar row, then presses its tab's close button (`Close tab <name>` in AXDescription); a pinned tab has none.
func orcaCloseTab(name string) error {
	if err := orcaPressRow(name); err != nil {
		return err
	}
	app, err := orcaAXApp()
	if err != nil {
		return err
	}
	defer C.ax_release(app)
	cRole, cAttr, cText := C.CString("AXButton"), C.CString("AXDescription"), C.CString("Close tab "+name)
	defer C.free(unsafe.Pointer(cRole))
	defer C.free(unsafe.Pointer(cAttr))
	defer C.free(unsafe.Pointer(cText))
	deadline := time.Now().Add(orcaMenuWait)
	for {
		var truncated C.int
		if b := C.ax_find_attr(app, cRole, cAttr, cText, C.AX_MATCH_EXACT, axMaxNodes, &truncated); b != 0 {
			code := C.ax_press(b)
			C.ax_release(b)
			if code != 0 {
				return fmt.Errorf("AXPress on close of %q: AXError %d", name, int(code))
			}
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("no close button for %q after %s (pinned tab?)", name, orcaMenuWait)
		}
		time.Sleep(orcaTabEvery)
	}
}

const orcaMenuWait = time.Second

// orcaMarkRead opens the context menu of the chat's sidebar row and presses `Mark Read`; a menu with `Mark Unread` means already read and is closed.
func orcaMarkRead(name string) error {
	app, err := orcaAXApp()
	if err != nil {
		return err
	}
	defer C.ax_release(app)
	row, err := orcaRow(app, name)
	if err != nil {
		return err
	}
	defer C.ax_release(row)
	cShow := C.CString("AXShowMenu")
	defer C.free(unsafe.Pointer(cShow))
	if code := C.orca_action(row, cShow); code != 0 {
		return fmt.Errorf("AXShowMenu on row %q: AXError %d", name, int(code))
	}
	cRole, cAttr, cRead, cUnread := C.CString("AXMenuItem"), C.CString("AXTitle"), C.CString("Mark Read"), C.CString("Mark Unread")
	defer C.free(unsafe.Pointer(cRole))
	defer C.free(unsafe.Pointer(cAttr))
	defer C.free(unsafe.Pointer(cRead))
	defer C.free(unsafe.Pointer(cUnread))
	deadline := time.Now().Add(orcaMenuWait)
	for {
		var truncated C.int
		if item := C.ax_find_attr(app, cRole, cAttr, cRead, C.AX_MATCH_EXACT, axMaxNodes, &truncated); item != 0 {
			code := C.ax_press(item)
			C.ax_release(item)
			if code != 0 {
				return fmt.Errorf("AXPress on Mark Read of %q: AXError %d", name, int(code))
			}
			return nil
		}
		if item := C.ax_find_attr(app, cRole, cAttr, cUnread, C.AX_MATCH_EXACT, axMaxNodes, &truncated); item != 0 {
			code := C.orca_cancel_menu(item)
			C.ax_release(item)
			if code != 0 {
				return fmt.Errorf("close the menu of %q: AXError %d", name, int(code))
			}
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("no Mark Read / Mark Unread in the menu of %q after %s", name, orcaMenuWait)
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
// orcaRichLines counts the wrapped lines of orcaMarkdown output.
func orcaRichLines(text string, w, size float64) int {
	s := C.CString(text)
	defer C.free(unsafe.Pointer(s))
	return int(C.cal_rich_lines(s, C.double(w), C.double(size)))
}

func orcaStripPNG(w, h, bar float64, texts ...calText) (string, error) {
	c := C.cal_canvas_new(C.double(w), C.double(h), 4, 1)
	for _, t := range texts {
		if t.rich {
			s := C.CString(t.text)
			C.cal_canvas_rich(c, s, C.double(t.x), C.double(t.y), C.double(t.w), C.double(t.h), C.double(t.size), C.uint(t.rgb), whiteRGB)
			C.free(unsafe.Pointer(s))
			continue
		}
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
