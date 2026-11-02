package main

/*
#include "ax_darwin.h"
*/
import "C"

import (
	"errors"
	"log"
	"os/exec"
	"sync"
	"time"
)

const (
	dictateIdle      = "Press and hold to record"
	dictateRecording = "Stop dictation"
	dictatePollEvery = time.Second
)

var errNoDictation = errors.New("dictation button not found")

type dictation struct {
	mu   sync.Mutex
	vars *vars
}

func findDictation(w C.AXUIElementRef) (C.AXUIElementRef, bool) {
	if e := findAttr(w, "AXCheckBox", "AXDescription", dictateRecording, false); e != 0 {
		return e, true
	}
	return findAttr(w, "AXCheckBox", "AXDescription", dictateIdle, false), false
}

func (d *dictation) toggle() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	pid := claudePid()
	if pid == 0 {
		return errors.New("Claude is not running")
	}
	if C.ax_frontmost_pid() != pid {
		if out, err := exec.Command("/usr/bin/open", "-b", claudeBundle).CombinedOutput(); err != nil {
			return errors.New("activate Claude: " + err.Error() + ": " + string(out))
		}
	}
	var was bool
	err := inClaudeWindow(pid, func(w C.AXUIElementRef) error {
		var e C.AXUIElementRef
		e, was = findDictation(w)
		if e == 0 {
			return errNoDictation
		}
		defer C.ax_release(e)
		if code := C.ax_press(e); code != 0 {
			return errors.New("press dictation: AX error")
		}
		return nil
	})
	if err != nil {
		return err
	}
	if waitFor(func() bool { return d.read() != was }) != nil {
		return errors.New("dictation did not switch")
	}
	return nil
}

func (d *dictation) read() bool {
	on := false
	pid := claudePid()
	if pid != 0 {
		err := inClaudeWindow(pid, func(w C.AXUIElementRef) error {
			var e C.AXUIElementRef
			e, on = findDictation(w)
			if e != 0 {
				C.ax_release(e)
			}
			return nil
		})
		if err != nil && !errors.Is(err, errNoWindow) {
			log.Printf("dictation: %v", err)
		}
	}
	d.vars.set("dictation", map[bool]string{true: "on", false: "off"}[on])
	return on
}

func (d *dictation) poll() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.read()
}
