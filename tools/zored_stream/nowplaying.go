package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	nowPlayingRestart = time.Second
	titleHold         = 3 * time.Second
	yandexMusic       = "ru.yandex.desktop.music"
	svgFont           = 100
	stripFont         = "JetBrains Mono"
	stripScaleX       = 0.85
	svgHeight         = 250
	svgMinWidth       = svgHeight * 270 / 60
	stripMaxRunes     = 23
	heartPath         = "M12 21.35l-1.45-1.32C5.4 15.36 2 12.28 2 8.5 2 5.42 4.42 3 7.5 3c1.74 0 3.41.81 4.5 2.09C13.09 3.81 14.76 3 16.5 3 19.58 3 22 5.42 22 8.5c0 3.78-3.4 6.86-8.55 11.54L12 21.35z"
)

var hearts = map[string]string{
	"liked":    `<path fill="#ff3b6b" d="` + heartPath + `"/>`,
	"disliked": `<path fill="#888" d="` + heartPath + `"/><path fill="none" stroke="#000" stroke-width="1.5" d="M12 5l-2 5 4 3-3 8"/>`,
}

type track struct {
	Title    string  `json:"title"`
	Artist   string  `json:"artist"`
	Playing  bool    `json:"playing"`
	App      string  `json:"app"`
	Artwork  string  `json:"artwork"`
	Duration float64 `json:"duration"`
	Elapsed  float64 `json:"elapsed"`
	Rate     float64 `json:"rate"`
	At       float64 `json:"at"`
}

func (t track) position(now time.Time) float64 {
	elapsed := t.Elapsed
	if t.Playing && t.At > 0 {
		elapsed += (float64(now.UnixNano())/1e9 - t.At) * t.Rate
	}
	return max(0, min(t.Duration, elapsed))
}

type nowPlaying struct {
	script, lib string
	mu          sync.Mutex
	current     track
	last        track
	heldUntil   time.Time
	like        string
	likeTitle   string
	vars        *vars
	glyph       *glyph
	tint        *tint
	commands    io.Writer
}

func (n *nowPlaying) run() {
	for {
		err := n.session()
		log.Printf("now playing helper exited: %v", err)
		time.Sleep(nowPlayingRestart)
	}
}

func (n *nowPlaying) session() error {
	cmd := exec.Command("/usr/bin/perl", n.script, n.lib)
	cmd.Stderr = os.Stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	n.mu.Lock()
	n.commands = in
	n.mu.Unlock()
	defer func() {
		n.mu.Lock()
		n.commands = nil
		n.mu.Unlock()
		in.Close()
	}()
	lines := bufio.NewScanner(out)
	lines.Buffer(make([]byte, 64*1024), 1024*1024)
	for lines.Scan() {
		var t track
		if err := json.Unmarshal(lines.Bytes(), &t); err != nil {
			log.Printf("now playing: %v: %s", err, lines.Bytes())
			continue
		}
		n.update(t)
	}
	if err := lines.Err(); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return err
	}
	return cmd.Wait()
}

func (n *nowPlaying) update(t track) {
	n.mu.Lock()
	if t.Title == "" && n.current.Title != "" {
		n.heldUntil = time.Now().Add(titleHold)
		time.AfterFunc(titleHold, n.publish)
	}
	if t.Title != "" {
		n.last = t
	}
	n.current = t
	n.mu.Unlock()
	n.publish()
}

func (n *nowPlaying) publish() {
	n.mu.Lock()
	t, like := n.current, n.like
	if t.Title == "" && time.Now().Before(n.heldUntil) {
		t.Title, t.Artist = n.last.Title, n.last.Artist
	}
	if !sameTitle(t.Title, n.likeTitle) {
		like = ""
	}
	n.mu.Unlock()
	playing := "0"
	if t.Playing {
		playing = "1"
	}
	trackText := t.Title
	if t.Artist != "" && t.Title != "" {
		trackText = t.Artist + " — " + t.Title
	}
	n.vars.set("np_title", t.Title)
	n.vars.set("np_artist", t.Artist)
	n.vars.set("np_track", trackText)
	n.vars.set("np_svg", trackSVG(t, like))
	n.vars.set("np_playing", playing)
	n.vars.set("np_app", t.App)
	n.vars.set("np_art", coverURI(t))
	if err := n.tint.setArtwork(t.Artwork); err != nil {
		log.Printf("tint: %v", err)
	}
	n.glyph.setPlaying(t.Playing)
	n.publishProgress()
}

func (n *nowPlaying) publishProgress() {
	t := n.snapshot()
	progress := 0.0
	if t.Duration > 0 {
		progress = t.position(time.Now()) / t.Duration * 100
	}
	n.vars.set("np_progress", fmt.Sprintf("%.1f", progress))
}

func (n *nowPlaying) snapshot() track {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.current
}

func (n *nowPlaying) seek(seconds float64) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.commands == nil {
		return errors.New("now playing helper is not running")
	}
	n.current.Elapsed, n.current.At = seconds, float64(time.Now().UnixNano())/1e9
	_, err := fmt.Fprintf(n.commands, "seek %.3f\n", seconds)
	return err
}

func sameTitle(a, b string) bool {
	return a != "" && b != "" && (strings.HasPrefix(a, b) || strings.HasPrefix(b, a))
}

func (n *nowPlaying) setLike(title, state string) {
	n.mu.Lock()
	changed := n.like != state || n.likeTitle != title
	n.like, n.likeTitle = state, title
	n.mu.Unlock()
	if changed {
		n.publish()
	}
}

type svgLine struct {
	markup string
	runes  int
}

func span(attr, text string) svgLine {
	return svgLine{fmt.Sprintf(`<tspan %s>%s</tspan>`, attr, html.EscapeString(text)), utf8.RuneCountInString(text)}
}

func join(lines ...svgLine) svgLine {
	out := svgLine{}
	for _, l := range lines {
		out.markup += l.markup
		out.runes += l.runes
	}
	return out
}

func lineWidth(l svgLine) int {
	return int(float64(l.runes*svgFont*60/100) * stripScaleX * 1.16)
}

func shorten(s string) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= stripMaxRunes {
		return string(r)
	}
	return strings.TrimSpace(string(r[:stripMaxRunes-1])) + "…"
}

func trackSVG(t track, like string) string {
	heart := ""
	if t.App == yandexMusic {
		heart = hearts[like]
	}
	return stripSVG(t.Artist, t.Title, "#fff", heart)
}

func stripSVG(gray, bold, fill, heart string) string {
	artist := span(`fill="#888"`, shorten(strings.ToLower(gray)))
	title := span(`font-weight="bold"`, shorten(strings.ToLower(bold)))
	lines := []svgLine{join(artist, title)}
	if gray != "" && bold != "" {
		lines = []svgLine{join(artist, span(`fill="#888"`, " | "), title)}
		if lineWidth(lines[0]) > svgMinWidth {
			lines = []svgLine{artist, title}
		}
	}
	pad := 0
	if heart != "" {
		inset := svgHeight * 6 / 100
		size := svgHeight - 2*inset
		pad = size + 2*inset
		heart = fmt.Sprintf(`<g transform="translate(%d %d) scale(%.2f) rotate(-90 12 12)">%s</g>`,
			inset, inset, float64(size)/24, heart)
	}
	width := svgMinWidth
	for _, l := range lines {
		width = max(width, lineWidth(l)+pad)
	}
	texts := ""
	for i, l := range lines {
		center := svgHeight * (2*i + 1) / (2 * len(lines))
		texts += fmt.Sprintf(`<text transform="translate(%d %d) scale(%.2f 1)">%s</text>`,
			pad+(width-pad)/2, center+svgFont*35/100, stripScaleX, l.markup)
	}
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d">%s<g text-anchor="middle" `+
		`font-family="%s" font-size="%d" fill="%s">%s</g></svg>`, width, svgHeight, heart, stripFont, svgFont, fill, texts)
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
}

func (n *nowPlaying) app() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.current.App
}
