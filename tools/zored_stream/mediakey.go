package main

import (
	"log"
	"net/http"
)

var mediaKeys = map[string]int{"volup": 0, "voldown": 1, "mute": 7, "play": 16, "next": 17, "prev": 18}

func serveKey(post func(int) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		key, ok := mediaKeys[name]
		if !ok {
			http.Error(w, "unknown media key "+name, http.StatusNotFound)
			return
		}
		if err := post(key); err != nil {
			log.Printf("/key/%s: %v", name, err)
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
