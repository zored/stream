package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func keyMux(post func(int) error) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /key/{name}", serveKey(post))
	return mux
}

func TestServeKey(t *testing.T) {
	var got []int
	mux := keyMux(func(k int) error { got = append(got, k); return nil })
	for _, c := range []struct {
		name string
		code int
	}{{"play", 204}, {"next", 204}, {"prev", 204}, {"bogus", 404}} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("POST", "/key/"+c.name, nil))
		if rec.Code != c.code {
			t.Errorf("%s: code %d, want %d", c.name, rec.Code, c.code)
		}
	}
	if len(got) != 3 || got[0] != 16 || got[1] != 17 || got[2] != 18 {
		t.Errorf("posted %v", got)
	}
}

func TestServeKeyError(t *testing.T) {
	mux := keyMux(func(int) error { return errors.New("no ax") })
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/key/play", nil))
	if rec.Code != http.StatusConflict {
		t.Errorf("code %d, want 409", rec.Code)
	}
}
