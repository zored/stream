package main

/*
#cgo LDFLAGS: -framework ApplicationServices -framework AppKit
#include <stdlib.h>
#include "ax_darwin.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"
	"sync"
	"time"
	"unsafe"
)

const (
	claudeBundle = "com.anthropic.claudefordesktop"
	aiSettle     = 400 * time.Millisecond
	aiWait       = 1500 * time.Millisecond
	aiStep       = 100 * time.Millisecond
	aiPollEvery  = time.Second
	aiPreview    = 5 * time.Second
	stripHeight  = 270.0
	keyE         = 14
	keyEscape    = 53
)

var (
	aiModels  = []string{"Sonnet", "Opus", "Fable"}
	aiEfforts = []string{"Low", "Medium", "High"}

	errNotFront   = errors.New("Claude is not frontmost")
	errNoNewChat  = errors.New("not on the new-chat screen")
	errNoPicker   = errors.New("model picker not found")
	errNoMenuItem = errors.New("menu item not found")
)

type aiChoice struct{ model, effort string }

type aiSwitch struct {
	mu      sync.Mutex
	index   int
	pending bool
	timer   *time.Timer
	applyMu sync.Mutex
	vars    *vars
	held    bool
	turned  bool
	preview time.Time
	onShow  func(stripLabel)
}

func newAISwitch(v *vars, onShow func(stripLabel)) *aiSwitch {
	return &aiSwitch{index: 1, vars: v, onShow: onShow}
}

func (a *aiSwitch) target() aiChoice {
	return aiChoice{aiModels[a.index/len(aiEfforts)], aiEfforts[a.index%len(aiEfforts)]}
}

func (a *aiSwitch) show(c aiChoice, state string) {
	a.vars.set("ai_model", c.model)
	a.vars.set("ai_effort", c.effort)
	a.vars.set("ai_state", state)
	a.onShow(stripLabel{c.model, c.effort, state})
}

func (a *aiSwitch) turn(d int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.held {
		a.turned = true
		n := len(aiEfforts)
		a.index = min(max(a.index/n+d, 0), len(aiModels)-1)*n + a.index%n
	} else {
		a.index = min(max(a.index+d, 0), len(aiModels)*len(aiEfforts)-1)
	}
	a.schedule()
}

func (a *aiSwitch) slide(y float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	steps := len(aiModels) * len(aiEfforts)
	index := min(max(int((stripHeight-y)/stripHeight*float64(steps)), 0), steps-1)
	if index == a.index && a.pending {
		return
	}
	a.index = index
	a.schedule()
}

func (a *aiSwitch) down() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.held, a.turned = true, false
	return nil
}

func (a *aiSwitch) up() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.held = false
	if !a.turned {
		a.schedule()
	}
	return nil
}

func (a *aiSwitch) schedule() {
	a.pending = true
	a.show(a.target(), "pending")
	if a.timer != nil {
		a.timer.Stop()
	}
	a.timer = time.AfterFunc(aiSettle, a.applyPending)
}

func (a *aiSwitch) applyPending() {
	a.mu.Lock()
	index, want := a.index, a.target()
	a.mu.Unlock()
	err := a.apply(want)
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.index != index {
		return
	}
	a.pending = false
	switch {
	case err == nil:
		a.show(want, "ok")
	case errors.Is(err, errNoNewChat):
		a.preview = time.Now().Add(aiPreview)
		a.show(want, "preview")
	case errors.Is(err, errNotFront):
		log.Printf("ai: %v", err)
		a.show(want, "idle")
	default:
		log.Printf("ai: apply %s %s: %v", want.model, want.effort, err)
		a.show(want, "error")
	}
}

func (a *aiSwitch) poll() {
	a.mu.Lock()
	busy := a.pending || time.Now().Before(a.preview)
	a.mu.Unlock()
	if busy {
		return
	}
	var cur aiChoice
	err := withClaude(false, func(_ C.pid_t, w C.AXUIElementRef) (err error) {
		cur, err = readChoice(w)
		return err
	})
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.pending || time.Now().Before(a.preview) {
		return
	}
	if err != nil {
		if !errors.Is(err, errNotFront) && !errors.Is(err, errNoPicker) {
			log.Printf("ai: poll: %v", err)
		}
		a.show(a.target(), "idle")
		return
	}
	m, e := slices.Index(aiModels, cur.model), slices.Index(aiEfforts, cur.effort)
	if m < 0 || e < 0 {
		a.show(cur, "other")
		return
	}
	a.index = m*len(aiEfforts) + e
	a.show(cur, "ok")
}

func (a *aiSwitch) apply(want aiChoice) error {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()
	return withNewChat(func(pid C.pid_t, w C.AXUIElementRef) error {
		cur, err := readChoice(w)
		if err != nil {
			return err
		}
		if cur.model != want.model {
			if err := pressFound(w, "AXPopUpButton", "AXDescription", "Model: "); err != nil {
				return err
			}
			if err := pick(pid, w, want.model+" ", func(c aiChoice) bool { return c.model == want.model }); err != nil {
				return fmt.Errorf("model %s: %w", want.model, err)
			}
		}
		if cur.effort != want.effort {
			C.ax_key(pid, keyE, C.AX_CMD|C.AX_SHIFT)
			if err := pick(pid, w, want.effort, func(c aiChoice) bool { return c.effort == want.effort }); err != nil {
				return fmt.Errorf("effort %s: %w", want.effort, err)
			}
		}
		return nil
	})
}

func withNewChat(f func(pid C.pid_t, window C.AXUIElementRef) error) error {
	return withClaude(true, f)
}

func withClaude(newChatOnly bool, f func(pid C.pid_t, window C.AXUIElementRef) error) error {
	bundle := C.CString(claudeBundle)
	defer C.free(unsafe.Pointer(bundle))
	pid := C.ax_app_pid(bundle)
	if pid == 0 || C.ax_frontmost_pid() != pid {
		return errNotFront
	}
	app := C.ax_app(pid)
	defer C.ax_release(app)
	w := C.ax_focused_window(app)
	if w == 0 {
		return errNotFront
	}
	defer C.ax_release(w)
	if newChatOnly {
		area := findAttr(w, "AXWebArea", "AXTitle", "Claude", false)
		if area == 0 {
			return errNoNewChat
		}
		C.ax_release(area)
	}
	return f(pid, w)
}

func findAttr(root C.AXUIElementRef, role, attr, text string, prefix bool) C.AXUIElementRef {
	cRole, cAttr, cText := C.CString(role), C.CString(attr), C.CString(text)
	defer C.free(unsafe.Pointer(cRole))
	defer C.free(unsafe.Pointer(cAttr))
	defer C.free(unsafe.Pointer(cText))
	p := C.int(0)
	if prefix {
		p = 1
	}
	var truncated C.int
	found := C.ax_find_attr(root, cRole, cAttr, cText, p, axMaxNodes, &truncated)
	if truncated != 0 {
		log.Printf("ai: AX walk stopped at %d nodes", axMaxNodes)
	}
	return found
}

func axString(e C.AXUIElementRef, attr string) string {
	cAttr := C.CString(attr)
	defer C.free(unsafe.Pointer(cAttr))
	s := C.ax_string(e, cAttr)
	if s == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(s))
	return C.GoString(s)
}

func readChoice(w C.AXUIElementRef) (aiChoice, error) {
	popup := findAttr(w, "AXPopUpButton", "AXDescription", "Model: ", true)
	if popup == 0 {
		return aiChoice{}, errNoPicker
	}
	defer C.ax_release(popup)
	desc := axString(popup, "AXDescription")
	fields := strings.Fields(strings.TrimPrefix(desc, "Model: "))
	if len(fields) < 2 {
		return aiChoice{}, fmt.Errorf("unexpected picker description %q", desc)
	}
	return aiChoice{fields[0], fields[len(fields)-1]}, nil
}

func pressFound(w C.AXUIElementRef, role, attr, text string) error {
	e := findAttr(w, role, attr, text, true)
	if e == 0 {
		return fmt.Errorf("%s %q not found", role, text)
	}
	defer C.ax_release(e)
	if err := C.ax_press(e); err != 0 {
		return fmt.Errorf("press %s %q: AX error %d", role, text, err)
	}
	return nil
}

func pick(pid C.pid_t, w C.AXUIElementRef, title string, done func(aiChoice) bool) error {
	if err := waitFor(func() bool {
		e := findAttr(w, "AXMenuItem", "AXTitle", title, true)
		if e == 0 {
			return false
		}
		defer C.ax_release(e)
		return C.ax_press(e) == 0
	}); err != nil {
		C.ax_key(pid, keyEscape, 0)
		return errNoMenuItem
	}
	if waitFor(func() bool { c, err := readChoice(w); return err == nil && done(c) }) == nil {
		return nil
	}
	C.ax_key(pid, keyEscape, 0)
	if c, err := readChoice(w); err != nil || !done(c) {
		return fmt.Errorf("picker shows %+v after pressing %q (%v)", c, title, err)
	}
	return nil
}

func waitFor(ok func() bool) error {
	for deadline := time.Now().Add(aiWait); time.Now().Before(deadline); time.Sleep(aiStep) {
		if ok() {
			return nil
		}
	}
	return errors.New("timeout")
}
