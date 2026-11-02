package main

import (
	"bufio"
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"
)

const nowPlayingRestart = time.Second

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
	t := n.current
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
	n.vars.set("np_playing", playing)
	n.vars.set("np_app", t.App)
}
