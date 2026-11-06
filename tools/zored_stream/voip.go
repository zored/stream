package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	voipPollEvery = 150 * time.Millisecond
	voipHistory   = 12 * time.Hour
	voipKeyGap    = 400 * time.Millisecond
)

type voipKind int

const (
	voipStart voipKind = iota
	voipMic
	voipCam
	voipMedia
	voipEnd
)

type voipEvent struct {
	kind     voipKind
	callID   string
	mic, cam bool
}

var (
	voipStartRe = regexp.MustCompile(`VM_Start(?:Outgoing|Incoming): id:([0-9a-f-]+)`)
	voipTxRe    = regexp.MustCompile(`OnStateMedia(Audio|Video)TXEnabled: call_id: ([0-9a-f-]+) enabled=([01])`)
	voipMediaRe = regexp.MustCompile(`CallObserversWithCurrentState: .*_media_audio_enable_tx:([01]), _media_video_enable_tx:([01])`)
	voipEndRe   = regexp.MustCompile(`\(VoipController\.cc:\d+\): OnCallTerminated: `)
)

func parseVoipLine(line string) (voipEvent, bool) {
	if m := voipStartRe.FindStringSubmatch(line); m != nil {
		return voipEvent{kind: voipStart, callID: m[1]}, true
	}
	if m := voipTxRe.FindStringSubmatch(line); m != nil {
		kind := voipMic
		if m[1] == "Video" {
			kind = voipCam
		}
		return voipEvent{kind: kind, callID: m[2], mic: m[3] == "1", cam: m[3] == "1"}, true
	}
	if m := voipMediaRe.FindStringSubmatch(line); m != nil {
		return voipEvent{kind: voipMedia, mic: m[1] == "1", cam: m[2] == "1"}, true
	}
	if voipEndRe.MatchString(line) {
		return voipEvent{kind: voipEnd}, true
	}
	return voipEvent{}, false
}

type voipState struct {
	callID  string
	media   [2]bool
	known   [2]bool
	started bool
}

func (s *voipState) apply(e voipEvent) {
	switch e.kind {
	case voipStart:
		if s.callID != e.callID {
			*s = voipState{callID: e.callID}
		}
		s.started = true
	case voipMedia:
		if s.callID != "" {
			s.media, s.known = [2]bool{e.mic, e.cam}, [2]bool{true, true}
		}
	case voipMic, voipCam:
		if s.callID != e.callID {
			*s = voipState{callID: e.callID}
		}
		i := 0
		if e.kind == voipCam {
			i = 1
		}
		s.media[i], s.known[i] = e.mic, true
	case voipEnd:
		*s = voipState{}
	}
}

func voipDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "Application Support", "VK WorkSpace", "logs"), nil
}

func voipFiles(dir string, since time.Time) ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "voip_*.log"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var out []string
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if st.ModTime().After(since) {
			out = append(out, p)
		}
	}
	return out, nil
}

func voipNewest(dir string) (string, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "voip_*.log"))
	if err != nil || len(paths) == 0 {
		return "", err
	}
	sort.Strings(paths)
	return paths[len(paths)-1], nil
}

type voipTail struct {
	path   string
	offset int64
	rest   []byte
}

func (t *voipTail) read(each func(string)) error {
	f, err := os.Open(t.path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Seek(t.offset, io.SeekStart); err != nil {
		return err
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	t.offset += int64(len(b))
	b = append(t.rest, b...)
	for {
		i := bytes.IndexByte(b, '\n')
		if i < 0 {
			break
		}
		each(string(b[:i]))
		b = b[i+1:]
	}
	t.rest = append([]byte(nil), b...)
	return nil
}

func (d *call) runVoip() {
	dir, err := voipDir()
	if err != nil {
		log.Printf("voip: %v", err)
		return
	}
	var st voipState
	tail := &voipTail{}
	files, err := voipFiles(dir, time.Now().Add(-voipHistory))
	if err != nil {
		log.Printf("voip: %v", err)
	}
	for _, p := range files {
		tail = &voipTail{path: p}
		if err := tail.read(func(line string) {
			if e, ok := parseVoipLine(line); ok {
				st.apply(e)
			}
		}); err != nil {
			log.Printf("voip catch-up %s: %v", p, err)
		}
	}
	st.started = false
	d.applyVoip(st)
	for {
		time.Sleep(voipPollEvery)
		if st.callID != "" {
			pid, err := vkwsPid()
			if err != nil {
				log.Printf("voip: %v", err)
			} else if pid == 0 {
				st = voipState{}
				d.applyVoip(st)
			}
		}
		newest, err := voipNewest(dir)
		if err != nil {
			log.Printf("voip: %v", err)
			continue
		}
		if newest == "" {
			continue
		}
		changed := false
		each := func(line string) {
			if e, ok := parseVoipLine(line); ok {
				st.apply(e)
				changed = true
			}
		}
		if newest != tail.path {
			if tail.path != "" {
				if err := tail.read(each); err != nil {
					log.Printf("voip %s: %v", tail.path, err)
				}
			}
			tail = &voipTail{path: newest}
		}
		if err := tail.read(each); err != nil {
			log.Printf("voip %s: %v", tail.path, err)
			continue
		}
		if changed {
			d.applyVoip(st)
			if st.known == [2]bool{true, true} {
				st.started = false
			}
		}
	}
}

func (d *call) applyVoip(st voipState) {
	d.mu.Lock()
	if st.callID == "" {
		if d.voipID != "" {
			log.Printf("voip call %q ended", d.voipID)
			d.voipID = ""
			d.callID = ""
			d.stopConfirmLocked()
			d.setLocked(none, none)
		}
		d.mu.Unlock()
		return
	}
	if d.voipID != st.callID {
		log.Printf("voip call %q", st.callID)
		d.voipID, d.callID = st.callID, st.callID
		d.stopConfirmLocked()
		d.setLocked(unknown, unknown)
	}
	d.voipMedia = st.media
	if st.known != [2]bool{true, true} {
		d.mu.Unlock()
		return
	}
	d.mu.Unlock()
	d.setMedia(st.callID, st.media[0], st.media[1])
	if st.started && (st.media[0] || st.media[1]) {
		log.Printf("voip call %q: new call, muting", st.callID)
		if err := d.change(func(bool, bool) (bool, bool) { return false, false }); err != nil {
			log.Printf("voip auto-mute: %v", err)
		}
	}
}

func (d *call) voipChangeLocked(mic, cam bool) error {
	pid, err := vkwsPid()
	if err != nil {
		return err
	}
	if pid == 0 {
		return fmt.Errorf("VK WorkSpace is not running")
	}
	base := d.voipMedia
	if d.confirm != nil {
		base = d.want
	}
	if mic != base[0] {
		voipKey(pid, keyA)
	}
	if cam != base[1] {
		if mic != base[0] {
			time.Sleep(voipKeyGap)
		}
		voipKey(pid, keyV)
	}
	return nil
}

func (d *call) voipHangUpLocked() error {
	pid, err := vkwsPid()
	if err != nil {
		return err
	}
	if pid == 0 {
		return fmt.Errorf("VK WorkSpace is not running")
	}
	id, mic, cam := d.voipID, d.mic, d.cam
	d.stopConfirmLocked()
	d.setLocked(pending, pending)
	go func() {
		start := time.Now()
		err := voipCloseCallWindow(pid)
		if err == nil {
			log.Printf("voip call %q: hung up in %s", id, time.Since(start).Round(time.Millisecond))
			return
		}
		log.Printf("voip call %q: hang up: %v", id, err)
		d.mu.Lock()
		defer d.mu.Unlock()
		if d.voipID == id {
			d.setLocked(mic, cam)
		}
	}()
	return nil
}

func isVkwsProcess(comm string) bool {
	return strings.HasPrefix(comm, "VK WorkSpace")
}
