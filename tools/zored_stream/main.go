package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	companionVars = "http://127.0.0.1:8000/api/custom-variable/"
	pollEvery     = 250 * time.Millisecond
	resendEvery   = 10 * time.Second
)

var client = &http.Client{Timeout: time.Second}

type vars struct {
	mu   sync.Mutex
	sent map[string]string
}

func (v *vars) set(name, value string) {
	v.mu.Lock()
	if v.sent[name] == value {
		v.mu.Unlock()
		return
	}
	v.sent[name] = value
	v.mu.Unlock()
	if err := post(name, value); err != nil {
		log.Printf("set %s: %v", name, err)
		v.mu.Lock()
		delete(v.sent, name)
		v.mu.Unlock()
	}
}

func (v *vars) forget() {
	v.mu.Lock()
	clear(v.sent)
	v.mu.Unlock()
}

func post(name, value string) error {
	resp, err := client.Post(companionVars+name+"/value", "application/json", strings.NewReader(strconv.Quote(value)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s: %s", resp.Status, body)
	}
	return nil
}

func handle(mux *http.ServeMux, path string, action func() error) {
	mux.HandleFunc("POST "+path, func(w http.ResponseWriter, _ *http.Request) {
		if err := action(); err != nil {
			log.Printf("%s: %v", path, err)
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func every(d time.Duration, f func()) {
	for range time.Tick(d) {
		f()
	}
}

func serve(addr string, register func(*http.ServeMux)) {
	mux := http.NewServeMux()
	register(mux)
	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	callAddr := flag.String("call", "127.0.0.1:47114", "VK WorkSpace call HTTP address")
	volumeAddr := flag.String("volume", "127.0.0.1:47113", "volume HTTP address")
	ymusicAddr := flag.String("ymusic", "127.0.0.1:47115", "Yandex Music like HTTP address")
	usageAddr := flag.String("usage", "127.0.0.1:47116", "Claude usage HTTP address")
	aiAddr := flag.String("ai", "127.0.0.1:47117", "Claude Desktop model + effort HTTP address")
	usageEvery := flag.Duration("usage-every", 15*time.Minute, "Claude usage fetch interval")
	usageProxy := flag.String("usage-proxy", "", "HTTP proxy for api.anthropic.com")
	npScript := flag.String("nowplaying-script", filepath.Join(home, ".local/share/zored-stream/nowplaying.pl"), "perl loader for the Now Playing dylib")
	npLib := flag.String("nowplaying-lib", filepath.Join(home, ".local/lib/libnowplaying.dylib"), "Now Playing dylib")
	flag.Parse()
	log.SetPrefix("zored_stream: ")

	c := &call{mic: unknown, cam: unknown, changed: make(chan struct{}, 1)}
	go c.publish()
	go c.run()
	go serve(*callAddr, func(mux *http.ServeMux) {
		handle(mux, "/mic/down", c.micDown)
		handle(mux, "/mic/up", c.micUp)
		handle(mux, "/cam/down", c.camDown)
		handle(mux, "/call/end", c.hangUp)
	})

	v := &vars{sent: map[string]string{}}
	np := &nowPlaying{script: *npScript, lib: *npLib, vars: v}
	go np.run()
	ym := newYmusic(np, v)
	go every(pollEvery, ym.poll)
	go serve(*ymusicAddr, func(mux *http.ServeMux) {
		handle(mux, "/like", ym.pressAndPoll(true))
		handle(mux, "/dislike", ym.pressAndPoll(false))
	})
	us, err := newUsage(v, *usageEvery, *usageProxy)
	if err != nil {
		log.Fatal(err)
	}
	go us.run()
	go us.animateStrip()
	go serve(*usageAddr, func(mux *http.ServeMux) {
		mux.HandleFunc("GET /usage", us.serveJSON)
		handle(mux, "/usage/refresh", us.refresh)
	})
	ai := newAISwitch(v, us.focus)
	go every(aiPollEvery, ai.poll)
	go serve(*aiAddr, func(mux *http.ServeMux) {
		mux.HandleFunc("POST /ai/turn", func(w http.ResponseWriter, r *http.Request) {
			d, err := strconv.Atoi(r.URL.Query().Get("d"))
			if err != nil {
				http.Error(w, "d: "+err.Error(), http.StatusBadRequest)
				return
			}
			ai.turn(d)
			w.WriteHeader(http.StatusNoContent)
		})
		mux.HandleFunc("POST /ai/slide", func(w http.ResponseWriter, r *http.Request) {
			y, err := strconv.ParseFloat(r.URL.Query().Get("y"), 64)
			if err != nil {
				http.Error(w, "y: "+err.Error(), http.StatusBadRequest)
				return
			}
			ai.slide(y)
			w.WriteHeader(http.StatusNoContent)
		})
		handle(mux, "/ai/down", ai.down)
		handle(mux, "/ai/up", ai.up)
	})
	vol, err := newVolume(v)
	if err != nil {
		log.Fatal(err)
	}
	vol.publish()
	go vol.animate()
	go every(pollEvery, func() {
		if err := vol.poll(); err != nil {
			log.Fatal(err)
		}
	})
	go every(resendEvery, func() {
		v.forget()
		vol.publish()
		np.publish()
		us.publish()
	})
	serve(*volumeAddr, func(mux *http.ServeMux) {
		handle(mux, "/volume/up", func() error { return vol.step(1) })
		handle(mux, "/volume/down", func() error { return vol.step(-1) })
		handle(mux, "/volume/mute", vol.toggleMute)
		mux.HandleFunc("POST /volume/drag", func(w http.ResponseWriter, r *http.Request) {
			dy, err := strconv.ParseFloat(r.URL.Query().Get("dy"), 64)
			if err != nil {
				http.Error(w, "dy: "+err.Error(), http.StatusBadRequest)
				return
			}
			if err := vol.drag(dy); err != nil {
				log.Printf("/volume/drag: %v", err)
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	})
}
