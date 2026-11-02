package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"math"
	"strconv"
	"sync"
	"time"
)

const (
	tintGray       = 0x38
	tintValue      = 0.28
	tintMaxSat     = 0.55
	tintMinSat     = 0.25
	tintHueBuckets = 12
	tintEasing     = 0.06
)

type rgb [3]float64

var defaultTint = rgb{tintGray, tintGray, tintGray}

type tint struct {
	mu     sync.Mutex
	source string
	target rgb
	shown  rgb
	wake   chan struct{}
	vars   *vars
}

func newTint(v *vars) *tint {
	return &tint{target: defaultTint, shown: defaultTint, wake: make(chan struct{}, 1), vars: v}
}

func (t *tint) setArtwork(artwork string) error {
	t.mu.Lock()
	if artwork == t.source {
		t.mu.Unlock()
		return nil
	}
	t.source = artwork
	t.mu.Unlock()
	target := defaultTint
	if artwork != "" {
		accent, err := coverAccent(artwork)
		if err != nil {
			return err
		}
		target = accent
	}
	t.mu.Lock()
	t.target = target
	t.mu.Unlock()
	select {
	case t.wake <- struct{}{}:
	default:
	}
	return nil
}

func coverAccent(artwork string) (rgb, error) {
	data, err := base64.StdEncoding.DecodeString(artwork)
	if err != nil {
		return rgb{}, fmt.Errorf("cover: %w", err)
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return rgb{}, fmt.Errorf("cover: %w", err)
	}
	return accent(img), nil
}

func accent(img image.Image) rgb {
	var weight [tintHueBuckets]float64
	var sum [tintHueBuckets]rgb
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			c := rgb{float64(r >> 8), float64(g >> 8), float64(bl >> 8)}
			h, s, v := hsv(c)
			w := s * v
			if s < tintMinSat || v < 0.15 {
				continue
			}
			i := int(h/360*tintHueBuckets) % tintHueBuckets
			weight[i] += w
			for k := range c {
				sum[i][k] += c[k] * w
			}
		}
	}
	best := 0
	for i := range weight {
		if weight[i] > weight[best] {
			best = i
		}
	}
	if weight[best] == 0 {
		return defaultTint
	}
	var mean rgb
	for k := range mean {
		mean[k] = sum[best][k] / weight[best]
	}
	h, s, _ := hsv(mean)
	return fromHSV(h, min(s, tintMaxSat), tintValue)
}

func hsv(c rgb) (h, s, v float64) {
	r, g, b := c[0]/255, c[1]/255, c[2]/255
	hi, lo := max(r, g, b), min(r, g, b)
	v = hi
	if hi == 0 {
		return 0, 0, 0
	}
	s = (hi - lo) / hi
	d := hi - lo
	switch {
	case d == 0:
		h = 0
	case hi == r:
		h = math.Mod((g-b)/d, 6)
	case hi == g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h, s, v
}

func fromHSV(h, s, v float64) rgb {
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := v - c
	var r, g, b float64
	switch int(h / 60) {
	case 0:
		r, g, b = c, x, 0
	case 1:
		r, g, b = x, c, 0
	case 2:
		r, g, b = 0, c, x
	case 3:
		r, g, b = 0, x, c
	case 4:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return rgb{(r + m) * 255, (g + m) * 255, (b + m) * 255}
}

func (t *tint) animate() {
	for range t.wake {
		ticker := time.NewTicker(time.Second / animationHz)
		for range ticker.C {
			if !t.frame() {
				break
			}
		}
		ticker.Stop()
	}
}

func (t *tint) frame() bool {
	t.mu.Lock()
	active := false
	for k := range t.shown {
		t.shown[k] += (t.target[k] - t.shown[k]) * tintEasing
		if math.Abs(t.target[k]-t.shown[k]) < 1 {
			t.shown[k] = t.target[k]
		}
		active = active || t.shown[k] != t.target[k]
	}
	t.mu.Unlock()
	t.publish()
	return active
}

func (t *tint) publish() {
	t.mu.Lock()
	c := t.shown
	t.mu.Unlock()
	t.vars.set("np_tint", strconv.Itoa(int(c[0])<<16|int(c[1])<<8|int(c[2])))
}
