package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	companionVars = "http://127.0.0.1:8000/api/custom-variable/"
	pollEvery     = 250 * time.Millisecond
	resendEvery   = 10 * time.Second
	progressEvery = 3 * time.Second
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

func handleValue(mux *http.ServeMux, path string, action func() (int, error)) {
	mux.HandleFunc("POST "+path, func(w http.ResponseWriter, _ *http.Request) {
		value, err := action()
		if err != nil {
			log.Printf("%s: %v", path, err)
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		fmt.Fprint(w, value)
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
	seekAddr := flag.String("seek", "127.0.0.1:47119", "Now Playing seek HTTP address")
	agentsAddr := flag.String("agents", "127.0.0.1:47120", "agents waiting for input HTTP address")
	agentsTTL := flag.Duration("agents-ttl", 4*time.Hour, "forget a waiting agent after this long")
	brightnessAddr := flag.String("brightness", "127.0.0.1:47118", "display + controller brightness HTTP address")
	scrollAddr := flag.String("scroll", "127.0.0.1:47121", "mouse wheel scroll HTTP address")
	scrollModeFile := flag.String("scroll-mode-file", filepath.Join(home, ".local/share/zored-stream/scroll-mode"), "scroll knob mode: wheel or tab")
	powerAddr := flag.String("power", "127.0.0.1:47122", "keep-awake + sleep HTTP address")
	companionDB := flag.String("companion-db", filepath.Join(home, "Library/Application Support/companion/v5.1/db.sqlite"), "Companion database with the surface brightness")
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
	np := &nowPlaying{script: *npScript, lib: *npLib, vars: v, glyph: newGlyph(v), tint: newTint(v)}
	go np.glyph.animate()
	np.tint.publish()
	go np.tint.animate()
	sk := newSkip(v)
	go sk.animate()
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
	dict := newDictation(v, np)
	go dict.run()
	go every(dictatePollEvery, dict.poll)
	go serve(*aiAddr, func(mux *http.ServeMux) {
		handle(mux, "/dictation/down", func() error { return dict.push(true) })
		handle(mux, "/dictation/up", func() error { return dict.push(false) })
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
	configDir, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	ag := newAgents(v, *agentsTTL, newDesktopSessions(configDir))
	ch := newChoice(v, func() (*choiceCard, choiceTarget, string, error) { return readChoiceCard(ag) })
	go ch.run()
	go every(choicePollEvery, ch.poll)
	ag.expire()
	ag.publish()
	go ag.animate()
	go every(agentCheckFreq, ag.expire)
	go serve(*agentsAddr, func(mux *http.ServeMux) {
		mux.HandleFunc("POST /agent/event", ag.serveEvent)
		mux.HandleFunc("GET /agents", ag.serveJSON)
		handle(mux, "/agent/focus", ag.focus)
		handle(mux, "/agent/new", newChat)
		mux.HandleFunc("GET /choice", ch.serveJSON)
		mux.HandleFunc("POST /choice/{edge}", func(w http.ResponseWriter, r *http.Request) {
			b, err := strconv.Atoi(r.URL.Query().Get("b"))
			if err != nil || b < 1 || b > choiceButtons {
				http.Error(w, "b: button 1-8", http.StatusBadRequest)
				return
			}
			press := map[string]func(int) error{"down": ch.down, "up": ch.up}[r.PathValue("edge")]
			if press == nil {
				http.Error(w, "edge: down or up", http.StatusNotFound)
				return
			}
			if err := press(b); err != nil {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	})
	vol, err := newVolume(v)
	if err != nil {
		log.Fatal(err)
	}
	vol.publish()
	vol.bar.publish()
	go vol.bar.animate()
	pw := newPower(v)
	pw.publish()
	br, err := newBrightness(vol.bar, *companionDB)
	if err != nil {
		log.Fatal(err)
	}
	go serve(*brightnessAddr, func(mux *http.ServeMux) {
		handleValue(mux, "/brightness/up", func() (int, error) {
			if pw.turn(1) {
				return br.level(), nil
			}
			return br.turn(1)
		})
		handleValue(mux, "/brightness/down", func() (int, error) {
			if pw.turn(-1) {
				return br.level(), nil
			}
			return br.turn(-1)
		})
		handleValue(mux, "/brightness/hold", br.hold)
		handleValue(mux, "/brightness/release", br.release)
	})
	seek := &seeker{np: np, bar: vol.bar}
	go every(progressEvery, np.publishProgress)
	go serve(*seekAddr, func(mux *http.ServeMux) {
		handle(mux, "/seek/forward", func() error { return seek.turn(1) })
		handle(mux, "/seek/back", func() error { return seek.turn(-1) })
	})
	sc, err := newScroller(*scrollModeFile)
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
		<-stop
		sc.release()
		os.Exit(0)
	}()
	go serve(*scrollAddr, func(mux *http.ServeMux) {
		handle(mux, "/scroll/down", func() error { return sc.hold(true) })
		handle(mux, "/scroll/up", func() error { return sc.hold(false) })
		handle(mux, "/scroll/forward", func() error { return sc.turn(1) })
		handle(mux, "/scroll/back", func() error { return sc.turn(-1) })
	})
	go every(time.Second, pw.publish)
	go every(awakePollEvery, pw.poll)
	go serve(*powerAddr, func(mux *http.ServeMux) {
		handle(mux, "/power/down", pw.down)
		handle(mux, "/power/up", pw.up)
	})
	go every(pollEvery, func() {
		if err := vol.poll(); err != nil {
			log.Fatal(err)
		}
	})
	go every(resendEvery, func() {
		v.forget()
		vol.publish()
		vol.bar.publish()
		np.publish()
		np.glyph.publish()
		np.tint.publish()
		us.publish()
		ag.publish()
		pw.publish()
	})
	serve(*volumeAddr, func(mux *http.ServeMux) {
		handle(mux, "/volume/up", func() error { return vol.step(1) })
		handle(mux, "/volume/down", func() error { return vol.step(-1) })
		handle(mux, "/volume/mute", vol.toggleMute)
		handle(mux, "/nowplaying/show", func() error { np.glyph.show(); return nil })
		handle(mux, "/nowplaying/next", func() error { sk.show(1); return nil })
		handle(mux, "/nowplaying/prev", func() error { sk.show(-1); return nil })
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
