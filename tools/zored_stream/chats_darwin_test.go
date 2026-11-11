package main

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"math/rand/v2"
	"strings"
	"testing"
	"time"
)

func TestChatsPhotoCardFitsCompanionBody(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 640, 640))
	r := rand.New(rand.NewPCG(1, 2))
	for i := range img.Pix {
		img.Pix[i] = uint8(r.IntN(256))
	}
	img.Set(0, 0, color.White)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	e := chatEntry{title: "x", unread: 3, at: time.Now(), avatar: buf.Bytes()}
	now := time.Now()
	for name, uri := range map[string]string{
		"card":  chatsPickImage([]chatEntry{e}, 0, 0, now),
		"slide": chatsSlideImage(e, e, 0.5, 1, now),
		"grid":  chatsTileImage([]chatEntry{e, e, e, e}, true, now),
	} {
		if len(uri) > 80_000 {
			t.Errorf("%s: %d bytes", name, len(uri))
		}
	}
}

func TestChatsTextStripScrolls(t *testing.T) {
	long := strings.Repeat("длинное сообщение ", 20)
	if n := chatsTextLines(long, stripDrawH-8, chatsTextSize); n < 6 {
		t.Fatalf("lines %d", n)
	}
	if chatsTextLines("short", stripDrawH-8, chatsTextSize) != 1 {
		t.Fatal("short text must be one line")
	}
	if chatsTextStrip(long, 0) == chatsTextStrip(long, chatsLineH) {
		t.Fatal("offset must move the text")
	}
}
