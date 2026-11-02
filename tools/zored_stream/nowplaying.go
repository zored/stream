package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	nowPlayingRestart = time.Second
	titleHold         = 3 * time.Second
	yandexMusic       = "ru.yandex.desktop.music"
	svgFont           = 100
	svgHeight         = 250
	svgMinWidth       = svgHeight * 270 / 60
	heartPath         = "M12 21.35l-1.45-1.32C5.4 15.36 2 12.28 2 8.5 2 5.42 4.42 3 7.5 3c1.74 0 3.41.81 4.5 2.09C13.09 3.81 14.76 3 16.5 3 19.58 3 22 5.42 22 8.5c0 3.78-3.4 6.86-8.55 11.54L12 21.35z"
)

var hearts = map[string]string{
	"liked":    `<path fill="#ff3b6b" d="` + heartPath + `"/>`,
	"disliked": `<path fill="#888" d="` + heartPath + `"/><path fill="none" stroke="#000" stroke-width="1.5" d="M12 5l-2 5 4 3-3 8"/>`,
}

type track struct {
	Title   string `json:"title"`
	Artist  string `json:"artist"`
	Playing bool   `json:"playing"`
	App     string `json:"app"`
}

type nowPlaying struct {
	script, lib string
	mu          sync.Mutex
	current     track
	last        track
	heldUntil   time.Time
	like        string
	vars        *vars
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
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
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
	if t.Title != "" && (t.Title != n.last.Title || t.Artist != n.last.Artist) {
		n.last, n.like = t, ""
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
}

func (n *nowPlaying) setLike(state string) {
	n.mu.Lock()
	changed := n.like != state
	n.like = state
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
	return l.runes * svgFont * 62 / 100 * 112 / 100
}

func trackSVG(t track, like string) string {
	heart := ""
	if t.App == yandexMusic {
		heart = hearts[like]
	}
	artist := span(`fill="#888"`, t.Artist)
	title := span(`font-weight="bold"`, t.Title)
	lines := []svgLine{join(artist, title)}
	if t.Artist != "" && t.Title != "" {
		lines = []svgLine{join(artist, span(`fill="#888"`, " | "), title)}
		if lineWidth(lines[0]) > svgMinWidth {
			lines = []svgLine{artist, title}
		}
	}
	pad := 0
	if heart != "" {
		pad = svgFont * 11 / 10
		heart = fmt.Sprintf(`<g transform="translate(%d %d) scale(%.2f)">%s</g>`,
			svgFont/10, svgHeight/2-svgFont*45/100, float64(svgFont)*9/10/24, heart)
	}
	width := svgMinWidth
	for _, l := range lines {
		width = max(width, lineWidth(l)+pad)
	}
	texts := ""
	for i, l := range lines {
		center := svgHeight * (2*i + 1) / (2 * len(lines))
		texts += fmt.Sprintf(`<text x="%d" y="%d">%s</text>`, pad+(width-pad)/2, center+svgFont*35/100, l.markup)
	}
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d">%s<g text-anchor="middle" `+
		`font-family="Helvetica" font-size="%d" fill="#fff">%s</g></svg>`, width, svgHeight, heart, svgFont, texts)
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
}

func (n *nowPlaying) app() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.current.App
}
