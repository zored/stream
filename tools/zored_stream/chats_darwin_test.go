package main

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"math/rand/v2"
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
		"card":  chatsPickImage([]chatEntry{e}, 0, now, false),
		"slide": chatsSlideImage(e, e, 0.5, 1, now, false),
		"grid":  chatsTileImage([]chatEntry{e, e, e, e}, true, now, false),
	} {
		if len(uri) > 80_000 {
			t.Errorf("%s: %d bytes", name, len(uri))
		}
	}
}
