package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const (
	choiceButtons   = 8
	choicePollEvery  = 500 * time.Millisecond
	choiceShownEvery = time.Second
	choicePicked    = 0x00FF00
	choiceLive      = 0xFF0000
	choicePrimary   = 0xFFFFFF
	choiceKeepPolls = 3
)

type choiceOption struct {
	Label    string
	Selected bool
	Primary  bool
}

type choiceCard struct {
	Options []choiceOption
	Multi   bool
	Index   int
	Count   int
	NoSkip  bool
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
	case b == 1 && c.NoSkip:
		return actNone, 0
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
		switch {
		case act == actChoose && c.Options[i].Selected:
			out[b-1] = choicePicked
		case act == actChoose && c.Options[i].Primary:
			out[b-1] = choicePrimary
		case act != actNone:
			out[b-1] = choiceLive
		}
	}
	return out
}

type choice struct {
	mu       sync.Mutex
	vars     *vars
	read     func() (*choiceCard, choiceTarget, string, error)
	why      string
	failures int
	card     *choiceCard
	target   choiceTarget
	consumed [choiceButtons + 1]bool
	presses  chan int
}

func newChoice(v *vars, read func() (*choiceCard, choiceTarget, string, error)) *choice {
	return &choice{vars: v, read: read, presses: make(chan int, 16)}
}

func (c *choice) poll() {
	card, target, why, err := c.read()
	c.mu.Lock()
	if err != nil {
		c.why = err.Error()
		c.failures++
		keep := c.failures <= choiceKeepPolls
		c.mu.Unlock()
		log.Printf("choice: %v (keeping the previous card: %v)", err, keep)
		if keep {
			return
		}
		c.mu.Lock()
	} else {
		c.failures = 0
	}
	if (card == nil) != (c.card == nil) {
		log.Printf("choice: card %v (%s)", card != nil, why)
	}
	c.card, c.target, c.why = card, target, why
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
		log.Printf("choice: button %d down with no card (%s)", b, c.why)
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

func (c *choice) showing() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.card != nil
}

func (c *choice) run() {
	for b := range c.presses {
		c.mu.Lock()
		card, target := c.card, c.target
		c.mu.Unlock()
		start := time.Now()
		if card != nil {
			if err := c.apply(*card, target, b); err != nil {
				log.Printf("choice: button %d: %v", b, err)
			}
		}
		applied := time.Since(start)
		c.poll()
		log.Printf("choice: button %d apply %v, repoll %v", b, applied, time.Since(start)-applied)
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

func (c *choice) serveJSON(w http.ResponseWriter, _ *http.Request) {
	c.mu.Lock()
	state := struct {
		Why  string      `json:"why"`
		Card *choiceCard `json:"card"`
	}{c.why, c.card}
	c.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(state); err != nil {
		log.Printf("choice json: %v", err)
	}
}
