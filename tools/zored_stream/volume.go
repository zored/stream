package main

import (
	"strconv"
	"sync"
)

const dragPercentPerPixel = 0.4

type volume struct {
	mu            sync.Mutex
	value         int
	muted         bool
	dragRemainder float64
	accel         accel
	bar           *bar
	vars          *vars
}

func newVolume(v *vars) (*volume, error) {
	value, muted, err := readAudio()
	if err != nil {
		return nil, err
	}
	vol := &volume{value: value, muted: muted, vars: v}
	vol.bar = newBar(v, "volume", float64(vol.levelLocked()))
	return vol, nil
}

func (v *volume) levelLocked() int {
	if v.muted {
		return 0
	}
	return v.value
}

func (v *volume) poll() error {
	value, muted, err := readAudio()
	if err != nil {
		return err
	}
	v.mu.Lock()
	changed := value != v.value || muted != v.muted
	if changed {
		v.value, v.muted = value, muted
		v.revealLocked()
	}
	v.mu.Unlock()
	if changed {
		v.publish()
	}
	return nil
}

func (v *volume) revealLocked() {
	v.bar.show("volume", float64(v.levelLocked()))
}

func (v *volume) publish() {
	v.mu.Lock()
	text := strconv.Itoa(v.value)
	if v.muted {
		text = "MUTE"
	}
	v.mu.Unlock()
	v.vars.set("volume", text)
}

func (v *volume) setLocked(value int) error {
	v.value = max(0, min(100, value))
	unmute := v.muted && value > 0
	if unmute {
		v.muted = false
	}
	v.revealLocked()
	if err := writeVolume(v.value); err != nil {
		return err
	}
	if unmute {
		return writeMute(false)
	}
	return nil
}

func (v *volume) step(direction int) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.setLocked(v.value + v.accel.step(direction))
}

func (v *volume) drag(pixels float64) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.dragRemainder += pixels * dragPercentPerPixel
	amount := int(v.dragRemainder)
	if amount == 0 {
		return nil
	}
	v.dragRemainder -= float64(amount)
	return v.setLocked(v.value + amount)
}

func (v *volume) toggleMute() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.muted = !v.muted
	v.revealLocked()
	return writeMute(v.muted)
}
