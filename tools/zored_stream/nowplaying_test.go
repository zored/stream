package main

import "testing"

func TestShorten(t *testing.T) {
	for in, want := range map[string]string{"get low": "get low", "lil jon & the east side": "lil jon & the east side", "lil jon & the east side boyz": "lil jon & the east sid…", "ab                            cd": "ab…"} {
		if got := shorten(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}
