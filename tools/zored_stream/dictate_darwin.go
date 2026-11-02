package main

/*
#include "ax_darwin.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	dictateIdle      = "Press and hold to record"
	dictateRecording = "Stop dictation"
	dictatePrompt    = "Prompt"
	dictatePollEvery = time.Second
	dictateHold      = 400 * time.Millisecond
	dictateSettle    = 700 * time.Millisecond
	dictateFinal     = 4 * time.Second
	keyReturn        = 36
)

var (
	errNoDictation = errors.New("dictation button not found")
	errNoClaude    = errors.New("Claude is not running")
)

type dictateEvent struct {
	down bool
	at   time.Time
}

type dictation struct {
	mu      sync.Mutex
	vars    *vars
	events  chan dictateEvent
	downAt  time.Time
	pressed bool
	started bool
}

func newDictation(v *vars) *dictation {
	return &dictation{vars: v, events: make(chan dictateEvent, 16)}
}

func (d *dictation) push(down bool) error {
	select {
	case d.events <- dictateEvent{down, time.Now()}:
		return nil
	default:
		return errors.New("dictation: event queue full")
	}
}

func (d *dictation) run() {
	for e := range d.events {
		d.mu.Lock()
		var err error
		if e.down {
			err = d.down(e.at)
		} else {
			err = d.up(e.at)
		}
		d.mu.Unlock()
		if err != nil {
			log.Printf("dictation: %v", err)
		}
	}
}

func (d *dictation) down(at time.Time) error {
	on, err := d.recording()
	if err != nil {
		return err
	}
	if !on {
		if err := d.switchTo(true); err != nil {
			return err
		}
	}
	d.downAt, d.pressed, d.started = at, true, !on
	return nil
}

func (d *dictation) up(at time.Time) error {
	if !d.pressed {
		return nil
	}
	d.pressed = false
	held := at.Sub(d.downAt) >= dictateHold
	switch {
	case d.started && !held:
		return nil
	case !d.started && held:
		return d.switchTo(false)
	}
	if err := d.switchTo(false); err != nil {
		return err
	}
	return d.send()
}

func withDictation(f func(pid C.pid_t, w C.AXUIElementRef) error) error {
	pid := claudePid()
	if pid == 0 {
		return errNoClaude
	}
	return inClaudeWindow(pid, func(w C.AXUIElementRef) error { return f(pid, w) })
}

func findDictation(w C.AXUIElementRef) (C.AXUIElementRef, bool) {
	if e := findAttr(w, "AXCheckBox", "AXDescription", dictateRecording, false); e != 0 {
		return e, true
	}
	return findAttr(w, "AXCheckBox", "AXDescription", dictateIdle, false), false
}

func (d *dictation) recording() (bool, error) {
	on := false
	err := withDictation(func(_ C.pid_t, w C.AXUIElementRef) error {
		e, rec := findDictation(w)
		if e == 0 {
			return errNoDictation
		}
		C.ax_release(e)
		on = rec
		return nil
	})
	d.vars.set("dictation", map[bool]string{true: "on", false: "off"}[on])
	return on, err
}

func (d *dictation) switchTo(want bool) error {
	pid := claudePid()
	if pid != 0 && C.ax_frontmost_pid() != pid {
		if out, err := exec.Command("/usr/bin/open", "-b", claudeBundle).CombinedOutput(); err != nil {
			return fmt.Errorf("activate Claude: %v: %s", err, out)
		}
	}
	err := withDictation(func(_ C.pid_t, w C.AXUIElementRef) error {
		e, on := findDictation(w)
		if e == 0 {
			return errNoDictation
		}
		defer C.ax_release(e)
		if on == want {
			return nil
		}
		if code := C.ax_press(e); code != 0 {
			return fmt.Errorf("press dictation: AX error %d", code)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if waitFor(func() bool { on, err := d.recording(); return err == nil && on == want }) != nil {
		return fmt.Errorf("dictation did not turn %v", map[bool]string{true: "on", false: "off"}[want])
	}
	return nil
}

func (d *dictation) send() error {
	return withDictation(func(pid C.pid_t, w C.AXUIElementRef) error {
		prompt := findAttr(w, "AXTextArea", "AXDescription", dictatePrompt, false)
		if prompt == 0 {
			return errors.New("prompt not found")
		}
		defer C.ax_release(prompt)
		text, changed := axString(prompt, "AXValue"), time.Now()
		for deadline := time.Now().Add(dictateFinal); time.Since(changed) < dictateSettle; time.Sleep(aiStep) {
			if time.Now().After(deadline) {
				return errors.New("transcript did not settle")
			}
			if now := axString(prompt, "AXValue"); now != text {
				text, changed = now, time.Now()
			}
		}
		if strings.TrimSpace(text) == "" {
			return errors.New("nothing dictated, not sending")
		}
		if code := C.ax_focus(prompt); code != 0 {
			return fmt.Errorf("focus prompt: AX error %d", code)
		}
		C.ax_key(pid, keyReturn, 0)
		if waitFor(func() bool { return strings.TrimSpace(axString(prompt, "AXValue")) == "" }) != nil {
			return errors.New("Return did not send the prompt")
		}
		return nil
	})
}

func (d *dictation) poll() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, err := d.recording(); err != nil && !errors.Is(err, errNoWindow) && !errors.Is(err, errNoDictation) &&
		!errors.Is(err, errNoClaude) {
		log.Printf("dictation: %v", err)
	}
}
