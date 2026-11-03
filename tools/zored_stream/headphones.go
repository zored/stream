package main

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

const (
	headphonesAddress  = "80:C3:BA:5F:A4:76"
	headphonesName     = "accentum"
	speakersName       = "logi"
	headphonesWait     = 10 * time.Second
	headphonesErrorFor = 3 * time.Second
	headphonesPoll     = 2 * time.Second
)

type audioDevice struct {
	id        uint32
	name      string
	transport string
	input     bool
	output    bool
}

type audioRoute struct {
	output, input audioDevice
}

type headphones struct {
	mu       sync.Mutex
	vars     *vars
	on       bool
	busy     bool
	errorTil time.Time
}

func newHeadphones(v *vars) *headphones {
	return &headphones{vars: v}
}

func (h *headphones) publish() {
	h.mu.Lock()
	defer h.mu.Unlock()
	stroke := "#fff"
	switch {
	case time.Now().Before(h.errorTil):
		stroke = "#f00"
	case h.busy:
		stroke = gray
	}
	h.vars.set("headphones_svg", headphonesSVG(h.on, stroke))
}

func (h *headphones) poll() {
	on, err := headphonesActive()
	if err != nil {
		log.Print(err)
		return
	}
	h.mu.Lock()
	if !h.busy {
		h.on = on
	}
	h.mu.Unlock()
	h.publish()
}

func (h *headphones) toggle() error {
	h.mu.Lock()
	if h.busy {
		h.mu.Unlock()
		return errors.New("headphones switch in progress")
	}
	h.busy = true
	on := h.on
	h.mu.Unlock()
	h.publish()
	go func() {
		err := switchHeadphones(!on)
		h.mu.Lock()
		h.busy = false
		if err != nil {
			log.Print(err)
			h.errorTil = time.Now().Add(headphonesErrorFor)
		}
		h.mu.Unlock()
		h.poll()
		time.AfterFunc(headphonesErrorFor, h.publish)
	}()
	return nil
}

func headphonesActive() (bool, error) {
	id, err := audioDefault(false)
	if err != nil {
		return false, err
	}
	devices, err := audioDevices()
	if err != nil {
		return false, err
	}
	for _, d := range devices {
		if d.id == id {
			return d.transport == "blue", nil
		}
	}
	return false, nil
}

func switchHeadphones(on bool) error {
	if !on {
		devices, err := audioDevices()
		if err != nil {
			return err
		}
		route, err := speakersRoute(devices)
		if err != nil {
			return err
		}
		if err := applyRoute(route); err != nil {
			return err
		}
		return btDisconnect(headphonesAddress)
	}
	connected, err := btConnected(headphonesAddress)
	if err != nil {
		return err
	}
	if !connected {
		if err := btConnect(headphonesAddress); err != nil {
			return err
		}
	}
	deadline := time.Now().Add(headphonesWait)
	for {
		devices, err := audioDevices()
		if err != nil {
			return err
		}
		route, err := headphonesRoute(devices)
		if err == nil {
			return applyRoute(route)
		}
		if time.Now().After(deadline) {
			return err
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func applyRoute(r audioRoute) error {
	if err := audioSetDefault(false, r.output.id); err != nil {
		return err
	}
	return audioSetDefault(true, r.input.id)
}

func isHeadphones(d audioDevice) bool {
	return d.transport == "blue" && strings.Contains(strings.ToLower(d.name), headphonesName)
}

func headphonesRoute(devices []audioDevice) (audioRoute, error) {
	out, okOut := findDevice(devices, func(d audioDevice) bool { return d.output && isHeadphones(d) })
	in, okIn := findDevice(devices, func(d audioDevice) bool { return d.input && isHeadphones(d) })
	if !okOut || !okIn {
		return audioRoute{}, fmt.Errorf("%s audio device not found", headphonesName)
	}
	return audioRoute{out, in}, nil
}

func speakersRoute(devices []audioDevice) (audioRoute, error) {
	out, ok := findDevice(devices, func(d audioDevice) bool {
		return d.output && strings.Contains(strings.ToLower(d.name), speakersName)
	})
	if !ok {
		out, ok = findDevice(devices, func(d audioDevice) bool { return d.output && d.transport == "bltn" })
	}
	if !ok {
		return audioRoute{}, errors.New("no speakers found")
	}
	in, ok := findDevice(devices, func(d audioDevice) bool { return d.input && d.transport == "usb " })
	if !ok {
		in, ok = findDevice(devices, func(d audioDevice) bool { return d.input && d.transport == "bltn" })
	}
	if !ok {
		return audioRoute{}, errors.New("no microphone found")
	}
	return audioRoute{out, in}, nil
}

func findDevice(devices []audioDevice, match func(audioDevice) bool) (audioDevice, bool) {
	for _, d := range devices {
		if match(d) {
			return d, true
		}
	}
	return audioDevice{}, false
}

func headphonesSVG(on bool, stroke string) string {
	glyph := `<rect x="30" y="18" width="40" height="64" rx="6"/><circle cx="50" cy="60" r="11"/><circle cx="50" cy="35" r="5"/>`
	if on {
		glyph = `<path d="M24 62V50a26 26 0 0 1 52 0v12"/><rect x="20" y="58" width="14" height="22" rx="5"/><rect x="66" y="58" width="14" height="22" rx="5"/>`
	}
	return dataURI(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">`+
		`<g stroke="%s" stroke-width="4" stroke-linejoin="round" fill="none">%s</g></svg>`, stroke, glyph))
}
