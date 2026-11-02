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
	"strings"
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

func trackSVG(t track, like string) string {
	mark := ""
	if t.App == yandexMusic {
		mark = map[string]string{"liked": "💖 ", "disliked": "💔 "}[like]
	}
	parts := []string{html.EscapeString(mark)}
	if t.Artist != "" {
		parts = append(parts, fmt.Sprintf(`<tspan fill="#888">%s</tspan>`, html.EscapeString(t.Artist)))
		if t.Title != "" {
			parts = append(parts, `<tspan fill="#888"> | </tspan>`)
		}
	}
	if t.Title != "" {
		parts = append(parts, fmt.Sprintf(`<tspan font-weight="bold">%s</tspan>`, html.EscapeString(t.Title)))
	}
	runes := utf8.RuneCountInString(mark + t.Artist + " | " + t.Title)
	width := max(svgMinWidth, runes*svgFont*62/100*112/100)
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d"><text x="%d" y="%d" `+
		`text-anchor="middle" font-family="Helvetica" font-size="%d" fill="#fff">%s</text></svg>`,
		width, svgHeight, width/2, svgHeight/2+svgFont*35/100, svgFont, strings.Join(parts, ""))
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
}

func (n *nowPlaying) app() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.current.App
}
