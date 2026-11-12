package main

import (
	"context"
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
	"sync/atomic"
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

var postVar = post

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
	if err := postVar(name, value); err != nil {
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
	log.Fatal(http.ListenAndServe(addr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path != "/agent/event" {
			touched.touch()
		}
		mux.ServeHTTP(w, r)
		if r.Method == http.MethodPost {
			tileModes.check()
			kickHints()
		}
	})))
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
	headphonesAddr := flag.String("headphones", "127.0.0.1:47123", "headphones toggle HTTP address")
	wifiVPN := flag.String("wifi-vpn", "Barbos_SE", "Viscosity connection to bring up after a network switch; empty = leave VPNs alone")
	wifiAddr := flag.String("wifi", "127.0.0.1:47124", "wi-fi power + network picker HTTP address")
	calendarAddr := flag.String("calendar", "127.0.0.1:47126", "calendar + reminders HTTP address")
	kvmAddr := flag.String("kvm", "127.0.0.1:47125", "monitor KVM input switch HTTP address")
	inputAddr := flag.String("input", "127.0.0.1:47127", "keyboard layout HTTP address")
	timersAddr := flag.String("timers", "127.0.0.1:47129", "timers + alarms HTTP address")
	timersFile := flag.String("timers-file", filepath.Join(home, ".config/zored-stream/timers.json"), "pending timers and alarms")
	emojiAddr := flag.String("emoji", "127.0.0.1:47128", "emoji picker HTTP address")
	switchAddr := flag.String("switch", "127.0.0.1:47132", "window switcher HTTP address")
	unreadAddr := flag.String("unread", "127.0.0.1:47130", "unread chats HTTP address")
	weatherAddr := flag.String("weather", "127.0.0.1:47131", "weather HTTP address")
	weatherLat := flag.Float64("weather-lat", 55.7558, "weather latitude")
	weatherLon := flag.Float64("weather-lon", 37.6173, "weather longitude")
	m1ddc := flag.String("m1ddc", "/opt/homebrew/bin/m1ddc", "DDC/CI CLI for the monitor input")
	kvmMAC := flag.String("kvm-mac", "ec:3a:56:fa:39:ac", "Windows PC to wake before switching")
	kvmWOL := flag.String("kvm-wol", "192.168.4.255:9", "Wake-on-LAN broadcast address")
	companionDB := flag.String("companion-db", filepath.Join(home, "Library/Application Support/companion/v5.1/db.sqlite"), "Companion database with the surface brightness")
	usageEvery := flag.Duration("usage-every", 15*time.Minute, "Claude usage fetch interval")
	usageProxy := flag.String("usage-proxy", "", "HTTP proxy for api.anthropic.com and Open-Meteo")
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
		handle(mux, "/peek/down", c.peekDown)
		handle(mux, "/peek/up", c.peekUp)
	})

	v := &vars{sent: map[string]string{}}
	hints := newKnobHints(v, readOrientation)
	overlay := newStripOverlay(v)
	em := newEmoji(v, overlay)
	em.publish()
	go em.animate()
	sw := newSwitcher(v, overlay)
	sw.publish()
	go sw.animate()
	go sw.work()
	go serve(*switchAddr, func(mux *http.ServeMux) {
		handle(mux, "/switch/tile", sw.pressTile)
	})
	pk := pickers{em, sw}
	go serve(*emojiAddr, func(mux *http.ServeMux) {
		handle(mux, "/emoji/tile", em.tile)
		mux.HandleFunc("POST /emoji/{edge}", func(w http.ResponseWriter, r *http.Request) {
			side := map[string]int{"l": emojiLeft, "r": emojiRight}
			s, ok := side[r.URL.Query().Get("s")]
			if !ok {
				http.Error(w, "s: l or r", http.StatusBadRequest)
				return
			}
			switch r.PathValue("edge") {
			case "start":
				y, err := strconv.ParseFloat(r.URL.Query().Get("y"), 64)
				if err != nil {
					http.Error(w, "y: "+err.Error(), http.StatusBadRequest)
					return
				}
				pk.start(s, y)
			case "end":
				if err := pk.end(s); err != nil {
					log.Printf("/emoji/end: %v", err)
					http.Error(w, err.Error(), http.StatusConflict)
					return
				}
			default:
				http.Error(w, "edge: start or end", http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	})
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
	var ag *agents
	var cs *chats
	go serve(*aiAddr, func(mux *http.ServeMux) {
		handle(mux, "/dictation/down", func() error { return dict.push(true) })
		handle(mux, "/dictation/up", func() error { return dict.push(false) })
		mux.HandleFunc("POST /ai/turn", func(w http.ResponseWriter, r *http.Request) {
			d, err := strconv.Atoi(r.URL.Query().Get("d"))
			if err != nil {
				http.Error(w, "d: "+err.Error(), http.StatusBadRequest)
				return
			}
			hints.turn("r1", d)
			switch {
			case cs.msgTurn(d):
			case ai.holding():
				ai.turn(d)
			default:
				ag.step(d)
			}
			w.WriteHeader(http.StatusNoContent)
		})
		mux.HandleFunc("POST /ai/slide", func(w http.ResponseWriter, r *http.Request) {
			y, err := strconv.ParseFloat(r.URL.Query().Get("y"), 64)
			if err != nil {
				http.Error(w, "y: "+err.Error(), http.StatusBadRequest)
				return
			}
			if !pk.slide(emojiRight, y) {
				ai.slide(y)
			}
			w.WriteHeader(http.StatusNoContent)
		})
		handle(mux, "/ai/down", func() error {
			hints.press("r1", true)
			return ai.down()
		})
		handle(mux, "/ai/up", func() error {
			hints.press("r1", false)
			if ai.up() {
				return nil
			}
			return ag.focus()
		})
	})
	configDir, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	ag = newAgents(v, *agentsTTL, newDesktopSessions(configDir))
	ag.strip = us.cover
	ch := newChoice(v, func() (*choiceCard, choiceTarget, string, error) { return readChoiceCard(ag) })
	go ch.run()
	var choicePolled time.Time
	go every(choicePollEvery, func() {
		if claude.due(&choicePolled) || ch.showing() && time.Since(choicePolled) >= choiceShownEvery {
			choicePolled = time.Now()
			ch.poll()
		}
	})
	ag.expire()
	ag.publish()
	go ag.animateSlide()
	go every(agentFrontFreq, ag.watchFront)
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
	hp := newHeadphones(v)
	hp.publish()
	kv, err := newKVM(v, *m1ddc, *kvmMAC, *kvmWOL)
	if err != nil {
		log.Fatal(err)
	}
	wf := newWifi(v, *wifiVPN, us.say)
	wf.poll()
	br, err := newBrightness(vol.bar, *companionDB)
	if err != nil {
		log.Fatal(err)
	}
	go serve(*brightnessAddr, func(mux *http.ServeMux) {
		handleValue(mux, "/brightness/up", func() (int, error) {
			hints.turn("r0", 1)
			if pw.turn(1) || wf.turn(1) || pk.turn(emojiRight, 1) {
				return br.level(), nil
			}
			return br.turn(1)
		})
		handleValue(mux, "/brightness/down", func() (int, error) {
			hints.turn("r0", -1)
			if pw.turn(-1) || wf.turn(-1) || pk.turn(emojiRight, -1) {
				return br.level(), nil
			}
			return br.turn(-1)
		})
		handleValue(mux, "/brightness/hold", func() (int, error) {
			hints.press("r0", true)
			if took, err := pk.press(emojiRight); took {
				return br.level(), err
			}
			return br.hold()
		})
		handleValue(mux, "/brightness/release", func() (int, error) {
			hints.press("r0", false)
			if pk.release(emojiRight) {
				return br.level(), nil
			}
			return br.release()
		})
	})
	tm, err := newTimers(v, *timersFile)
	if err != nil {
		log.Fatal(err)
	}
	go every(timerTick, tm.tick)
	go serve(*timersAddr, func(mux *http.ServeMux) {
		handle(mux, "/timer/down", tm.down)
		handle(mux, "/timer/up", tm.up)
	})
	seek := &seeker{np: np, bar: vol.bar}
	go every(progressEvery, np.publishProgress)
	go serve(*seekAddr, func(mux *http.ServeMux) {
		tk := &trackKnob{
			seek: seek.turn,
			skip: func(d int) error {
				sk.show(d)
				return postMediaKey(mediaKeys[map[int]string{1: "next", -1: "prev"}[d]])
			},
			play: func() error { return postMediaKey(mediaKeys["play"]) },
			like: ym.pressAndPoll(true),
		}
		handle(mux, "/seek/forward", func() error { return tk.turn(1) })
		handle(mux, "/seek/back", func() error { return tk.turn(-1) })
		handle(mux, "/track/down", tk.down)
		handle(mux, "/track/up", tk.up)
	})
	sc, err := newScroller(*scrollModeFile)
	if err != nil {
		log.Fatal(err)
	}
	tm.release = sc.release
	cal := newCalendar(v, sc.release, us.cover)
	go cal.poll()
	go every(calPollEvery, cal.poll)
	tile24 := &slot{vars: v, name: "tile_24", layers: []layer{
		{name: "call", active: c.inCall, press: c.hangUp},
		{name: "cal", press: cal.press},
	}}
	go every(time.Second, cal.publish)
	go every(pollEvery, tile24.publish)
	go serve(*calendarAddr, func(mux *http.ServeMux) {
		handle(mux, "/tile/24/press", tile24.press)
		handle(mux, "/button/5/press", func() error {
			if c.inCall() {
				return c.hangUp()
			}
			return cal.press()
		})
	})
	cs = newChats(v, sc.release, us.cover)
	go cs.animate()
	go cs.run(context.Background(), "tg", tgChats)
	go cs.run(context.Background(), "vkws", vkwsChats)
	go every(time.Second, cs.publish)
	tile22 := &slot{vars: v, name: "tile_22", layers: []layer{
		{name: "call", active: c.inCall, press: c.micDown, up: c.micUp},
		{name: "unread", press: cs.down, up: cs.up},
	}}
	go every(pollEvery, tile22.publish)
	wx, err := newWeather(v, *weatherLat, *weatherLon, *usageProxy)
	if err != nil {
		log.Fatal(err)
	}
	go wx.run()
	go every(time.Second, wx.publish)
	tile23 := &slot{vars: v, name: "tile_23", layers: []layer{
		{name: "call", active: c.inCall, press: c.camDown},
		{name: "weather", press: wx.press},
	}}
	go every(pollEvery, tile23.publish)
	hints.own("l2", func() bool { return wx.browsing() && !cs.capturing() })
	hints.own("r0", pw.sizing)
	hints.own("r0", wf.capturing)
	hints.own("r1", cs.capturing)
	hints.own("r2", tm.capturing)
	hints.own("r2", cal.capturing)
	hints.own("r2", cs.capturing)
	go hints.run()
	for _, md := range []mode{
		{"wifi", wf.capturing, wf.close},
		{"awake", pw.sizing, pw.close},
		{"emoji", em.isOpen, em.close},
		{"switch", sw.isOpen, sw.close},
		{"chats", cs.capturing, cs.close},
		{"weather", wx.browsing, wx.close},
		{"calendar", cal.capturing, cal.close},
		{"timer", tm.capturing, tm.close},
	} {
		tileModes.add(md)
	}
	go serve(*weatherAddr, func(mux *http.ServeMux) {
		handle(mux, "/tile/23/press", tile23.press)
	})
	go serve(*unreadAddr, func(mux *http.ServeMux) {
		handle(mux, "/tile/22/down", tile22.press)
		handle(mux, "/tile/22/up", tile22.up)
		var wxKnob atomic.Bool
		handle(mux, "/knob/down", func() error {
			hints.press("l2", true)
			if wx.browsing() {
				wxKnob.Store(true)
				return wx.press()
			}
			return cs.down()
		})
		handle(mux, "/knob/up", func() error {
			hints.press("l2", false)
			if wxKnob.Swap(false) {
				return nil
			}
			return cs.up()
		})
		knobTurn := func(direction int) error {
			hints.turn("l2", direction)
			if !cs.browsing() && wx.turn(direction) {
				return nil
			}
			return cs.knobTurn(direction)
		}
		handle(mux, "/knob/forward", func() error { return knobTurn(1) })
		handle(mux, "/knob/back", func() error { return knobTurn(-1) })
	})
	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
		<-stop
		sc.release()
		os.Exit(0)
	}()
	go serve(*scrollAddr, func(mux *http.ServeMux) {
		press := func(down bool) func() error {
			return func() error {
				hints.press("r2", down)
				if took, err := tm.knob(down); took {
					return err
				}
				if took, err := cal.knob(down); took {
					return err
				}
				if took, err := cs.knob(down); took {
					return err
				}
				return sc.hold(down)
			}
		}
		turn := func(direction int) func() error {
			return func() error {
				hints.turn("r2", direction)
				if tm.turn(direction) || cal.turn(direction) || cs.turn(direction) {
					return nil
				}
				return sc.turn(direction)
			}
		}
		handle(mux, "/scroll/down", press(true))
		handle(mux, "/scroll/up", press(false))
		handle(mux, "/scroll/forward", turn(1))
		handle(mux, "/scroll/back", turn(-1))
	})
	go every(time.Second, pw.publish)
	go every(awakePollEvery, pw.poll)
	go serve(*powerAddr, func(mux *http.ServeMux) {
		handle(mux, "/power/down", pw.down)
		handle(mux, "/power/up", pw.up)
	})
	go every(headphonesPoll, hp.poll)
	go serve(*headphonesAddr, func(mux *http.ServeMux) {
		handle(mux, "/headphones/toggle", hp.toggle)
	})
	in := &input{vars: v}
	in.publish()
	go serve(*inputAddr, func(mux *http.ServeMux) {
		handle(mux, "/input/next", inputNext)
	})
	go serve(*kvmAddr, func(mux *http.ServeMux) {
		handle(mux, "/kvm/windows", kv.windows)
		handle(mux, "/kvm/pip", kv.togglePIP)
	})
	go every(wifiPollEvery, wf.poll)
	go every(wifiProbeEvery, wf.probe)
	go serve(*wifiAddr, func(mux *http.ServeMux) {
		handle(mux, "/wifi/touch", wf.touch)
		handle(mux, "/wifi/power", wf.power)
	})
	if err := watchAudio(func() {
		if err := vol.poll(); err != nil {
			log.Fatal(err)
		}
	}); err != nil {
		log.Fatal(err)
	}
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
		hp.publish()
		wf.publish()
		kv.publish()
		cal.publish()
		tile24.publish()
		tile23.publish()
		wx.publish()
		tm.publish()
		in.publish()
		em.publish()
		sw.publish()
	})
	go serve(*volumeAddr, func(mux *http.ServeMux) {
		handle(mux, "/volume/up", func() error {
			if pk.turn(emojiLeft, 1) {
				return nil
			}
			return vol.step(1)
		})
		handle(mux, "/volume/down", func() error {
			if pk.turn(emojiLeft, -1) {
				return nil
			}
			return vol.step(-1)
		})
		handle(mux, "/volume/mute", func() error {
			if took, err := pk.press(emojiLeft); took {
				return err
			}
			return vol.toggleMute()
		})
		handle(mux, "/nowplaying/show", func() error { np.glyph.show(); return nil })
		handle(mux, "/nowplaying/next", func() error { sk.show(1); return nil })
		handle(mux, "/nowplaying/prev", func() error { sk.show(-1); return nil })
		ts := &tileSwipe{actions: map[string]func() error{
			"l": func() error { sk.show(-1); return postMediaKey(mediaKeys["prev"]) },
			"r": func() error { sk.show(1); return postMediaKey(mediaKeys["next"]) },
			"u": ym.pressAndPoll(true),
			"d": ym.pressAndPoll(false),
		}}
		mux.HandleFunc("POST /tile/swipe", ts.serve)
		handle(mux, "/nowplaying/tap", ts.tap(func() error { return postMediaKey(mediaKeys["play"]) }))
		mux.HandleFunc("POST /key/{name}", serveKey(postMediaKey))
		mux.HandleFunc("POST /volume/drag", func(w http.ResponseWriter, r *http.Request) {
			dy, err := strconv.ParseFloat(r.URL.Query().Get("dy"), 64)
			if err != nil {
				http.Error(w, "dy: "+err.Error(), http.StatusBadRequest)
				return
			}
			if pk.drag(emojiLeft, dy) {
				w.WriteHeader(http.StatusNoContent)
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
	inputWatch(in.publish)
}
