package main

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gotd/td/tg"
)

func photoPNG(t *testing.T) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 4, 2))
	for x := 0; x < 4; x++ {
		for y := 0; y < 2; y++ {
			img.Set(x, y, color.RGBA{uint8(x * 60), 0, 0, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestPhotoRGBCropsToFill(t *testing.T) {
	rgb, err := photoRGB(photoPNG(t), 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(rgb) != 2*2*3 {
		t.Fatalf("len=%d", len(rgb))
	}
	if rgb[0] < 50 || rgb[3] > 130 {
		t.Fatalf("not the center columns: %v", rgb)
	}
	if _, err := photoRGB([]byte("nope"), 2, 2); err == nil {
		t.Fatal("garbage must fail")
	}
}

func waitPhoto(t *testing.T, c *chats, id string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for c.photos.current().ID != id {
		if time.Now().After(deadline) {
			t.Fatalf("photo=%q, want %q", c.photos.current().ID, id)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestChatsScrollPastTextWalksPhotos(t *testing.T) {
	postVar = func(string, string) error { return nil }
	t.Cleanup(func() { postVar = post })
	c := newChats(&vars{sent: map[string]string{}}, func() {}, func(string) {})
	fetch := func(context.Context) ([]byte, error) { return []byte{1}, nil }
	ps := []chatPhoto{{key: "p1", fetch: fetch}, {key: "p2", fetch: fetch}}
	now := time.Now()
	c.set("tg", []chatEntry{{id: "a", unread: 2, at: now, msgs: []chatMsg{{text: "hi", photos: ps}, {text: "no photo"}}}})
	c.knobTurn(1)
	if !c.turn(1) || c.photo != 0 {
		t.Fatalf("photo=%d", c.photo)
	}
	waitPhoto(t, c, "p1")
	c.turn(1)
	c.turn(1)
	waitPhoto(t, c, "p2")
	c.turn(-1)
	c.turn(-1)
	waitPhoto(t, c, "")
	if c.photo != -1 {
		t.Fatalf("photo=%d", c.photo)
	}
	c.turn(1)
	waitPhoto(t, c, "p1")
	if err := c.closePhoto(); err != nil || c.photo != -1 || c.photos.current().ID != "" {
		t.Fatalf("photo=%d id=%q", c.photo, c.photos.current().ID)
	}
	c.turn(1)
	waitPhoto(t, c, "p1")
	c.msgTurn(1)
	waitPhoto(t, c, "")
	if c.turn(1); c.photo != -1 {
		t.Fatalf("a message without photos must not open one: %d", c.photo)
	}
	c.msgTurn(-1)
	c.turn(1)
	waitPhoto(t, c, "p1")
	c.blocked = func() bool { return true }
	c.publish()
	waitPhoto(t, c, "")
	c.blocked = func() bool { return false }
	c.close()
	waitPhoto(t, c, "")
}

func TestPhotoStateLongPoll(t *testing.T) {
	p := newPhotos()
	srv := httptest.NewServer(http.HandlerFunc(p.serveState))
	defer srv.Close()
	get := func(v int64) photoState {
		r, err := http.Get(srv.URL + "?v=" + strconv.FormatInt(v, 10))
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		var st photoState
		if err := json.NewDecoder(r.Body).Decode(&st); err != nil {
			t.Fatal(err)
		}
		return st
	}
	st := get(0)
	if st.ID != "" || len(st.Keep) != 1 || st.Keep[0] != (photoCell{2, 2}) {
		t.Fatalf("%+v", st)
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		p.show("x")
	}()
	start := time.Now()
	next := get(st.V)
	if next.ID != "x" || next.V == st.V || time.Since(start) < 40*time.Millisecond {
		t.Fatalf("%+v after %s", next, time.Since(start))
	}
}

func TestPhotoImageServesRawRGB(t *testing.T) {
	p := newPhotos()
	done := make(chan struct{})
	p.load(chatPhoto{key: "k", fetch: func(context.Context) ([]byte, error) { return photoPNG(t), nil }}, func() { close(done) })
	<-done
	srv := httptest.NewServer(http.HandlerFunc(p.serveImage))
	defer srv.Close()
	for q, want := range map[string]int{"?id=k&w=3&h=2": 200, "?id=z&w=3&h=2": 404, "?id=k&w=0&h=2": 400} {
		r, err := http.Get(srv.URL + q)
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r.Body)
		r.Body.Close()
		if r.StatusCode != want || want == 200 && buf.Len() != 3*2*3 {
			t.Fatalf("%s: %d, %d bytes", q, r.StatusCode, buf.Len())
		}
	}
}

func TestTgPhotoSizePicksSmallestThatFills(t *testing.T) {
	sizes := []tg.PhotoSizeClass{
		&tg.PhotoStrippedSize{Type: "i"},
		&tg.PhotoSize{Type: "m", W: 320, H: 240},
		&tg.PhotoSize{Type: "x", W: 800, H: 600},
		&tg.PhotoSizeProgressive{Type: "y", W: 1280, H: 960},
	}
	if s := tgPhotoSize(sizes); s != "x" {
		t.Fatalf("size=%q", s)
	}
	if s := tgPhotoSize(sizes[:2]); s != "m" {
		t.Fatalf("size=%q", s)
	}
	if s := tgPhotoSize(sizes[:1]); s != "" {
		t.Fatalf("size=%q", s)
	}
}

func TestVkwsHistoryImagesAndPreview(t *testing.T) {
	b := []byte(`{"status":{"code":20000},"results":{"yours":{"lastRead":1},"messages":[{"msgId":2,"text":"see","filesharing":[{"id":"f1","mime":"image/png"},{"id":"f2","mime":"application/pdf"}]}]}}`)
	d, err := parseVkwsHistory("a@x", b)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.msgs) != 1 || len(d.msgs[0].images) != 1 || d.msgs[0].images[0] != "f1" {
		t.Fatalf("%+v", d.msgs)
	}
	u, err := parseVkwsPreview([]byte(`{"status":{"code":20000},"result":{"previews":{"192":"s","800":"m"}}}`))
	if err != nil || u != "m" {
		t.Fatalf("%q %v", u, err)
	}
	if _, err := parseVkwsPreview([]byte(`{"status":{"code":20000},"result":{"previews":{}}}`)); err == nil {
		t.Fatal("no preview must fail")
	}
}
