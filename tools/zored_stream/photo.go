package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	photoPollFor  = 25 * time.Second
	photoFetchFor = 30 * time.Second
	photoCacheMax = 40
	photoSideMax  = 2000
	photoRetry    = 30 * time.Second
)

// photoKeep: tiles the fork leaves to Companion while a photo is up (тайл ряд 3, колонка 1 — the chats card).
var photoKeep = []photoCell{{Row: 2, Column: 2}}

type chatPhoto struct {
	key   string
	fetch func(ctx context.Context) ([]byte, error)
}

type photoCell struct {
	Row    int `json:"row"`
	Column int `json:"column"`
}

type photoState struct {
	V    int64       `json:"v"`
	ID   string      `json:"id"`
	Keep []photoCell `json:"keep"`
}

// photos holds the picture the fork spreads over the tiles: the fork long-polls the state, then asks the picture at its own size.
type photos struct {
	mu      sync.Mutex
	state   photoState
	change  chan struct{}
	cache   map[string][]byte
	order   []string
	loading map[string]bool
	failed  map[string]time.Time
}

func newPhotos() *photos {
	return &photos{state: photoState{V: time.Now().UnixMilli(), Keep: photoKeep}, change: make(chan struct{}),
		cache: map[string][]byte{}, loading: map[string]bool{}, failed: map[string]time.Time{}}
}

func (p *photos) show(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state.ID == id {
		return
	}
	p.state.ID = id
	p.state.V++
	close(p.change)
	p.change = make(chan struct{})
}

func (p *photos) current() photoState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

func (p *photos) cached(key string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.cache[key]
	return ok
}

// load fetches a photo once in the background; done runs after it lands.
func (p *photos) load(ph chatPhoto, done func()) {
	p.mu.Lock()
	if _, ok := p.cache[ph.key]; ok || p.loading[ph.key] || time.Since(p.failed[ph.key]) < photoRetry {
		p.mu.Unlock()
		return
	}
	p.loading[ph.key] = true
	p.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), photoFetchFor)
		defer cancel()
		start := time.Now()
		b, err := ph.fetch(ctx)
		p.mu.Lock()
		delete(p.loading, ph.key)
		if err != nil {
			p.failed[ph.key] = time.Now()
			p.mu.Unlock()
			log.Printf("photo %s: %v", ph.key, err)
			return
		}
		delete(p.failed, ph.key)
		p.cache[ph.key] = b
		p.order = append(p.order, ph.key)
		for len(p.order) > photoCacheMax {
			delete(p.cache, p.order[0])
			p.order = p.order[1:]
		}
		p.mu.Unlock()
		log.Printf("photo %s: %d bytes in %s", ph.key, len(b), time.Since(start).Round(time.Millisecond))
		done()
	}()
}

func (p *photos) serveState(w http.ResponseWriter, r *http.Request) {
	v, _ := strconv.ParseInt(r.URL.Query().Get("v"), 10, 64)
	p.mu.Lock()
	st, ch := p.state, p.change
	p.mu.Unlock()
	if st.V == v {
		select {
		case <-ch:
		case <-time.After(photoPollFor):
		case <-r.Context().Done():
			return
		}
		st = p.current()
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(st); err != nil {
		log.Printf("photo: state: %v", err)
	}
}

// serveImage answers raw RGB, w×h, the photo cropped to fill it.
func (p *photos) serveImage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	width, errW := strconv.Atoi(q.Get("w"))
	height, errH := strconv.Atoi(q.Get("h"))
	if errW != nil || errH != nil || width < 1 || height < 1 || width > photoSideMax || height > photoSideMax {
		http.Error(w, "w, h: 1.."+strconv.Itoa(photoSideMax), http.StatusBadRequest)
		return
	}
	p.mu.Lock()
	b, ok := p.cache[q.Get("id")]
	p.mu.Unlock()
	if !ok {
		http.Error(w, "no photo "+q.Get("id"), http.StatusNotFound)
		return
	}
	rgb, err := photoRGB(b, width, height)
	if err != nil {
		log.Printf("photo %s: %v", q.Get("id"), err)
		http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write(rgb)
}

func photoRGB(b []byte, width, height int) ([]byte, error) {
	src, format, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	sb := src.Bounds()
	if sb.Dx() == 0 || sb.Dy() == 0 {
		return nil, fmt.Errorf("empty %s", format)
	}
	crop := sb
	if sb.Dx()*height > sb.Dy()*width {
		cw := sb.Dy() * width / height
		crop.Min.X += (sb.Dx() - cw) / 2
		crop.Max.X = crop.Min.X + cw
	} else {
		ch := sb.Dx() * height / width
		crop.Min.Y += (sb.Dy() - ch) / 2
		crop.Max.Y = crop.Min.Y + ch
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, crop, draw.Src, nil)
	out := make([]byte, 0, width*height*3)
	for i := 0; i < len(dst.Pix); i += 4 {
		out = append(out, dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2])
	}
	return out, nil
}

func (p *photos) serve(mux *http.ServeMux, closePhoto func() error) {
	mux.HandleFunc("GET /photo", p.serveState)
	mux.HandleFunc("GET /photo/image", p.serveImage)
	handle(mux, "/photo/close", closePhoto)
}
