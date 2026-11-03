package main

import (
	"strings"
	"testing"
	"time"
)

func dictationWith(t track) (*dictation, *strings.Builder) {
	out := &strings.Builder{}
	return &dictation{np: &nowPlaying{current: t, commands: out}}, out
}

func (d *dictation) step(on bool) {
	d.music(on)
	d.was = on
}

func TestDictationMusic(t *testing.T) {
	song := track{App: "ru.yandex.desktop.music", Title: "a", Playing: true}
	paused := song
	paused.Playing = false
	other := paused
	other.Title = "b"
	for name, c := range map[string]struct {
		start   track
		during  track
		pauseAt time.Duration
		end     track
		want    string
	}{
		"pause and resume":         {song, paused, 0, paused, "pause\nplay\n"},
		"nothing playing":          {paused, paused, 0, paused, ""},
		"track changed":            {song, paused, 0, other, "pause\n"},
		"played meanwhile":         {song, song, 3 * time.Second, song, "pause\n"},
		"playing again at the end": {song, paused, 0, song, "pause\n"},
	} {
		d, out := dictationWith(c.start)
		d.step(true)
		d.pauseAt = d.pauseAt.Add(-c.pauseAt)
		d.np.current = c.during
		d.step(true)
		d.np.current = c.end
		d.step(false)
		if got := out.String(); got != c.want {
			t.Errorf("%s: got %q want %q", name, got, c.want)
		}
	}
}
