package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

const controllerMinBrightness = 10

type brightness struct {
	mu         sync.Mutex
	controller bool
	ctl        int
	accel      accel
	bar        *bar
}

func newBrightness(b *bar, companionDB string) (*brightness, error) {
	ctl, err := readControllerBrightness(companionDB)
	if err != nil {
		return nil, err
	}
	return &brightness{ctl: ctl, bar: b}, nil
}

func readControllerBrightness(db string) (int, error) {
	out, err := exec.Command("/usr/bin/sqlite3", "-readonly", db,
		"select json_extract(value, '$.config.brightness') from surfaces where id like 'loupedeck:%'").Output()
	if err != nil {
		return 0, fmt.Errorf("read controller brightness from %s: %w", db, err)
	}
	lines := strings.Fields(string(out))
	if len(lines) != 1 {
		return 0, fmt.Errorf("%s: want one loupedeck surface, got %q", db, out)
	}
	return strconv.Atoi(lines[0])
}

func (b *brightness) turn(direction int) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	amount := b.accel.step(direction)
	if b.controller {
		b.ctl = max(controllerMinBrightness, min(100, b.ctl+amount))
		b.bar.show("ctl", float64(b.ctl))
		return b.ctl, nil
	}
	mac, err := readDisplayBrightness()
	if err != nil {
		return 0, err
	}
	mac = max(0, min(100, mac+amount))
	if err := writeDisplayBrightness(mac); err != nil {
		return 0, err
	}
	b.bar.show("mac", float64(mac))
	return b.ctl, nil
}

func (b *brightness) hold() (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.controller = true
	b.bar.show("ctl", float64(b.ctl))
	return b.ctl, nil
}

func (b *brightness) release() (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.controller = false
	return b.ctl, nil
}
