package main

import (
	"testing"
	"time"
)

func TestTrackKnob(t *testing.T) {
	var got []string
	k := &trackKnob{
		seek: func(d int) error { got = append(got, map[int]string{1: "seek+", -1: "seek-"}[d]); return nil },
		skip: func(d int) error { got = append(got, map[int]string{1: "next", -1: "prev"}[d]); return nil },
		play: func() error { got = append(got, "play"); return nil },
		like: func() error { got = append(got, "like"); return nil },
	}
	k.turn(1)
	k.down()
	k.turn(-1)
	time.Sleep(trackKnobHold + 50*time.Millisecond)
	k.up()
	k.down()
	k.up()
	k.down()
	time.Sleep(trackKnobHold + 50*time.Millisecond)
	k.up()
	want := []string{"seek+", "prev", "play", "like"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
