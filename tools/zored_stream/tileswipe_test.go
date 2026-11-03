package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTileSwipe(t *testing.T) {
	var got []string
	ts := &tileSwipe{actions: map[string]func() error{"l": func() error { got = append(got, "prev"); return nil }}}
	plays := 0
	tap := ts.tap(func() error { plays++; return nil })

	for _, c := range []struct {
		url  string
		code int
	}{
		{"/tile/swipe?id=button-1-3&dir=l", http.StatusNotFound},
		{"/tile/swipe?id=button-1-2&dir=x", http.StatusBadRequest},
		{"/tile/swipe?id=button-1-2&dir=l", http.StatusNoContent},
	} {
		w := httptest.NewRecorder()
		ts.serve(w, httptest.NewRequest("POST", c.url, nil))
		if w.Code != c.code {
			t.Errorf("%s: %d, want %d", c.url, w.Code, c.code)
		}
	}
	if len(got) != 1 {
		t.Fatalf("actions: %v", got)
	}
	_ = tap()
	_ = tap()
	if plays != 1 {
		t.Errorf("plays = %d, want 1: the tap after a swipe is skipped", plays)
	}
}
