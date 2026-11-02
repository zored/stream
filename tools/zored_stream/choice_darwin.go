package main

/*
#include <stdlib.h>
#include "ax_darwin.h"
*/
import "C"

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unsafe"
)

const (
	itermBundle  = "com.googlecode.iterm2"
	choiceSettle = 300 * time.Millisecond
	itermTab     = "(ASCII character 9)"
	itermEsc     = "(ASCII character 27)"
	itermBackTab = `(ASCII character 27) & "[Z"`
)

var cardPosition = regexp.MustCompile(`^(\d+) of (\d+)$`)

func readChoiceCard(a *agents) (*choiceCard, choiceTarget, error) {
	front := C.ax_frontmost_pid()
	if pid := claudePid(); pid != 0 && front == pid {
		return readDesktopCard(pid)
	}
	bundle := C.CString(itermBundle)
	defer C.free(unsafe.Pointer(bundle))
	if iterm := C.ax_app_pid(bundle); iterm == 0 || front != iterm || !a.hasCLIChoice() {
		return nil, nil, nil
	}
	out, err := exec.Command("/usr/bin/osascript", "-e",
		`tell application id "`+itermBundle+`" to get unique id of current session of current window`).CombinedOutput()
	if err != nil {
		return nil, nil, fmt.Errorf("iTerm2 current session: %v: %s", err, out)
	}
	uid := strings.TrimSpace(string(out))
	card, id := a.cliCard(uid)
	if card == nil {
		return nil, nil, nil
	}
	return card, cliTarget{a: a, id: id, uid: uid}, nil
}

func readDesktopCard(pid C.pid_t) (*choiceCard, choiceTarget, error) {
	var dump string
	err := inClaudeWindow(pid, func(w C.AXUIElementRef) error {
		s := C.ax_card(w)
		if s != nil {
			dump = C.GoString(s)
			C.free(unsafe.Pointer(s))
		}
		return nil
	})
	if err != nil || dump == "" {
		return nil, nil, nil
	}
	card := &choiceCard{Index: 1, Count: 1}
	target := desktopTarget{}
	for _, line := range strings.Split(strings.TrimSpace(dump), "\n") {
		f := strings.Split(line, "\t")
		switch {
		case f[0] == "opt" && len(f) == 3:
			card.Options = append(card.Options, choiceOption{Label: f[1], Selected: f[2] == "1"})
			target.titles = append(target.titles, f[1])
		case f[0] == "btn" && (f[1] == "Next" || f[1] == "Submit"):
			target.forwardLabel = f[1]
		case f[0] == "txt" && strings.HasSuffix(f[1], " selected"):
			card.Multi = true
		case f[0] == "txt":
			if m := cardPosition.FindStringSubmatch(f[1]); m != nil {
				card.Index, _ = strconv.Atoi(m[1])
				card.Count, _ = strconv.Atoi(m[2])
			}
		}
	}
	if len(card.Options) == 0 || target.forwardLabel == "" {
		return nil, nil, fmt.Errorf("unexpected question card: %q", dump)
	}
	return card, target, nil
}

type desktopTarget struct {
	titles       []string
	forwardLabel string
}

func desktopPress(text string, selectOption bool) error {
	pid := claudePid()
	if pid == 0 {
		return errNoClaude
	}
	return inClaudeWindow(pid, func(w C.AXUIElementRef) error {
		cText := C.CString(text)
		defer C.free(unsafe.Pointer(cText))
		if code := C.ax_card_press(w, cText, C.int(map[bool]int{true: 1, false: 0}[selectOption])); code != 0 {
			return fmt.Errorf("press %q in the question card: %d", text, code)
		}
		return nil
	})
}

func (t desktopTarget) choose(card choiceCard, i int) error {
	if err := desktopPress(t.titles[i], !card.Multi); err != nil {
		return err
	}
	if card.Multi {
		return nil
	}
	time.Sleep(choiceSettle)
	return t.forward(card)
}

func (t desktopTarget) forward(choiceCard) error { return desktopPress(t.forwardLabel, false) }
func (t desktopTarget) back(choiceCard) error    { return desktopPress("Show previous question", false) }
func (t desktopTarget) skip(choiceCard) error    { return desktopPress("Skip", false) }

type cliTarget struct {
	a       *agents
	id, uid string
}

func (t cliTarget) write(expr string) error {
	return run(exec.Command("/usr/bin/osascript", "-e", fmt.Sprintf(itermWrite, t.uid, expr)))
}

func (t cliTarget) submitReview() error {
	time.Sleep(choiceSettle)
	return t.write(`"1"`)
}

func (t cliTarget) choose(card choiceCard, i int) error {
	if err := t.write(strconv.Quote(strconv.Itoa(i + 1))); err != nil {
		return err
	}
	if card.Multi {
		t.a.cliToggle(t.id, i)
		return nil
	}
	t.a.cliStep(t.id, 1)
	if card.Count > 1 && card.Index == card.Count {
		return t.submitReview()
	}
	return nil
}

func (t cliTarget) forward(card choiceCard) error {
	if err := t.write(itermTab); err != nil {
		return err
	}
	if card.Index < card.Count {
		t.a.cliStep(t.id, 1)
		return nil
	}
	return t.submitReview()
}

func (t cliTarget) back(choiceCard) error {
	if err := t.write(itermBackTab); err != nil {
		return err
	}
	t.a.cliStep(t.id, -1)
	return nil
}

func (t cliTarget) skip(choiceCard) error { return t.write(itermEsc) }

const itermWrite = `tell application id "com.googlecode.iterm2"
	repeat with w in windows
		repeat with t in tabs of w
			repeat with s in sessions of t
				if unique id of s is "%s" then
					tell s to write text %s newline NO
					return
				end if
			end repeat
		end repeat
	end repeat
	error "iTerm2 session not found"
end tell`
