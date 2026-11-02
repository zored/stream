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
	yandexMusic       = "ru.yandex.desktop.music"
	svgFont           = 100
	svgHeight         = 250
	svgMinWidth       = svgHeight * 270 / 60
)

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
		n.mu.Lock()
		n.current = t
		n.mu.Unlock()
		n.publish()
	}
	if err := lines.Err(); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return err
	}
	return cmd.Wait()
}

func (n *nowPlaying) publish() {
	n.mu.Lock()
	t, like := n.current, n.like
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
	mark := ""
	if t.App == yandexMusic {
		mark = map[string]string{"liked": "💖 ", "disliked": "💔 "}[like]
	}
	artist := join(span(`fill="#fff"`, mark), span(`fill="#888"`, t.Artist))
	title := span(`font-weight="bold"`, t.Title)
	lines := []svgLine{join(artist, title)}
	if t.Artist != "" && t.Title != "" {
		lines = []svgLine{join(artist, span(`fill="#888"`, " | "), title)}
		if lineWidth(lines[0]) > svgMinWidth {
			lines = []svgLine{artist, title}
		}
	}
	width := svgMinWidth
	for _, l := range lines {
		width = max(width, lineWidth(l))
	}
	texts := ""
	for i, l := range lines {
		center := svgHeight * (2*i + 1) / (2 * len(lines))
		texts += fmt.Sprintf(`<text x="%d" y="%d">%s</text>`, width/2, center+svgFont*35/100, l.markup)
	}
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d"><g text-anchor="middle" `+
		`font-family="Helvetica" font-size="%d" fill="#fff">%s</g></svg>`, width, svgHeight, svgFont, texts)
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
}

func (n *nowPlaying) app() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.current.App
}
