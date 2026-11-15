package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func stressN(t *testing.T, def int) int {
	s := os.Getenv("CAL_STRESS")
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatalf("CAL_STRESS=%q: %v", s, err)
	}
	return n
}

func TestCanvasConcurrentStress(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for i := range img.Pix {
		img.Pix[i] = uint8(i)
	}
	img.Set(0, 0, color.White)
	var avatar bytes.Buffer
	if err := png.Encode(&avatar, img); err != nil {
		t.Fatal(err)
	}
	texts := []calText{
		{text: "16:30 стендап", tail: " · 5м", x: 0, y: 24, w: 88, h: 22, size: 16, rgb: 0xffffff, tailRGB: 0x8e8e93},
		{text: strings.Repeat("длинное название встречи ", 6), x: 4, y: 4, w: 80, h: 60, size: 12, rgb: 0xffffff, center: true},
		{text: "", tail: "", x: 0, y: 0, w: 10, h: 10, size: 10},
		{text: "🙂👍🏽 emoji", tail: "x", x: 0, y: 50, w: 40, h: 40, size: 14, rgb: 0xff9f0a},
	}
	layers := []chatsLayer{
		{discs: []chatsDisc{{avatar: avatar.Bytes(), cx: 20, cy: 20, r: 18}, {cx: 60, cy: 20, r: 18, square: true}}, texts: texts, alpha: 1},
		{texts: texts, alpha: 0.4},
	}
	workers, rounds := 8, stressN(t, 50)
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for w := range workers {
		wg.Go(func() {
			for i := range rounds {
				uri, err := calPNG(90, 90, (w+i)%2 == 0, 0x1c1c1e, texts)
				if err != nil {
					errs <- err
					return
				}
				if !strings.HasPrefix(uri, "data:image/png;base64,") {
					t.Errorf("calPNG uri %.40q", uri)
					return
				}
				if uri := chatsLayersPNG(90, 90, layers); !strings.HasPrefix(uri, "data:image/jpeg;base64,") {
					t.Errorf("chats uri %.40q", uri)
					return
				}
				if n := chatsTextLines(texts[1].text, 80, 12); n < 2 {
					t.Errorf("text lines %d", n)
					return
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func TestCalFetchConcurrent(t *testing.T) {
	if os.Getenv("CAL_EVENTKIT") == "" {
		t.Skip("needs Calendars and Reminders access for the test binary: CAL_EVENTKIT=1")
	}
	now := time.Now()
	workers, rounds := 4, stressN(t, 10)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for range rounds {
				if _, err := calFetch(now.Add(-calHorizon), now.Add(calHorizon)); err != nil {
					t.Error(err)
					return
				}
			}
		})
	}
	wg.Wait()
}
