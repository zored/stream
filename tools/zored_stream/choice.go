package main

import (
	"errors"
	"log"
	"strconv"
	"sync"
	"time"
)

const (
	choiceButtons   = 8
	choicePollEvery = 500 * time.Millisecond
	choiceNav       = 0x606060
	choiceWhite     = 0xFFFFFF
	choiceOn        = 0x00FF00
	choiceOff       = 0xFF0000
)

type choiceOption struct {
	Label    string
	Selected bool
}

type choiceCard struct {
	Options []choiceOption
	Multi   bool
	Index   int
	Count   int
}

type choiceTarget interface {
	choose(card choiceCard, i int) error
	forward(card choiceCard) error
	back(card choiceCard) error
	skip(card choiceCard) error
}

type choiceAct int

const (
	actNone choiceAct = iota
	actBack
	actSkip
	actForward
	actChoose
)

func (c choiceCard) nav() bool {
	return c.Count > 1 || c.Multi
}

func (c choiceCard) action(b int) (choiceAct, int) {
	switch {
	case b == 1 && c.Index > 1:
		return actBack, 0
	case b == 1:
		return actSkip, 0
	case b == choiceButtons && c.nav():
		return actForward, 0
	case b-2 < len(c.Options):
		return actChoose, b - 2
	}
	return actNone, 0
}

func (c choiceCard) leds() [choiceButtons]int {
	var out [choiceButtons]int
	for b := 1; b <= choiceButtons; b++ {
		act, i := c.action(b)
		switch act {
		case actBack, actSkip:
			out[b-1] = choiceNav
		case actForward:
			out[b-1] = map[bool]int{true: choiceWhite, false: choiceNav}[c.Index >= c.Count]
		case actChoose:
			switch {
			case !c.Multi:
				out[b-1] = choiceWhite
			case c.Options[i].Selected:
				out[b-1] = choiceOn
			default:
				out[b-1] = choiceOff
			}
		}
	}
	return out
}

type choice struct {
	mu       sync.Mutex
	vars     *vars
	read     func() (*choiceCard, choiceTarget, error)
	card     *choiceCard
	target   choiceTarget
	consumed [choiceButtons + 1]bool
	presses  chan int
}

func newChoice(v *vars, read func() (*choiceCard, choiceTarget, error)) *choice {
	return &choice{vars: v, read: read, presses: make(chan int, 16)}
}

func (c *choice) poll() {
	card, target, err := c.read()
	if err != nil {
		log.Printf("choice: %v", err)
	}
	c.mu.Lock()
	c.card, c.target = card, target
	c.mu.Unlock()
	c.publish(card)
}

func (c *choice) publish(card *choiceCard) {
	if card == nil {
		c.vars.set("choice_on", "0")
		return
	}
	for i, color := range card.leds() {
		c.vars.set("choice_"+strconv.Itoa(i+1), strconv.Itoa(color))
	}
	c.vars.set("choice_on", "1")
}

func (c *choice) down(b int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.card == nil {
		return errors.New("no choice on screen")
	}
	select {
	case c.presses <- b:
	default:
		return errors.New("choice: press queue full")
	}
	c.consumed[b] = true
	return nil
}

func (c *choice) up(b int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.consumed[b] {
		return errors.New("press was not a choice")
	}
	c.consumed[b] = false
	return nil
}

func (c *choice) run() {
	for b := range c.presses {
		c.mu.Lock()
		card, target := c.card, c.target
		c.mu.Unlock()
		if card != nil {
			if err := c.apply(*card, target, b); err != nil {
				log.Printf("choice: button %d: %v", b, err)
			}
		}
		c.poll()
	}
}

func (c *choice) apply(card choiceCard, target choiceTarget, b int) error {
	act, i := card.action(b)
	switch act {
	case actBack:
		return target.back(card)
	case actSkip:
		return target.skip(card)
	case actForward:
		return target.forward(card)
	case actChoose:
		return target.choose(card, i)
	}
	return nil
}
