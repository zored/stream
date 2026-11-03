package main

import (
	"log"
	"net/http"
	"sync/atomic"
)

const playTile = "button-1-2"

type tileSwipe struct {
	swiped  atomic.Bool
	actions map[string]func() error
}

func (t *tileSwipe) serve(w http.ResponseWriter, r *http.Request) {
	if id := r.URL.Query().Get("id"); id != playTile {
		http.Error(w, "no swipes on "+id, http.StatusNotFound)
		return
	}
	dir := r.URL.Query().Get("dir")
	action, ok := t.actions[dir]
	if !ok {
		http.Error(w, "unknown dir "+dir, http.StatusBadRequest)
		return
	}
	t.swiped.Store(true)
	if err := action(); err != nil {
		log.Printf("/tile/swipe %s: %v", dir, err)
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (t *tileSwipe) tap(play func() error) func() error {
	return func() error {
		if t.swiped.Swap(false) {
			return nil
		}
		return play()
	}
}
