package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	companionVars = "http://127.0.0.1:8000/api/custom-variable/"
	listenAddr    = "127.0.0.1:47120"
	pollEvery     = 250 * time.Millisecond
	resendEvery   = 10 * time.Second
)

var client = &http.Client{Timeout: time.Second}

type vars struct {
	mu   sync.Mutex
	sent map[string]string
}

func (v *vars) set(name, value string) {
	v.mu.Lock()
	if v.sent[name] == value {
		v.mu.Unlock()
		return
	}
	v.sent[name] = value
	v.mu.Unlock()
	if err := post(name, value); err != nil {
		log.Printf("set %s: %v", name, err)
		v.mu.Lock()
		delete(v.sent, name)
		v.mu.Unlock()
	}
}

func (v *vars) forget() {
	v.mu.Lock()
	clear(v.sent)
	v.mu.Unlock()
}

func post(name, value string) error {
	resp, err := client.Post(companionVars+name+"/value", "application/json", strings.NewReader(strconv.Quote(value)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s: %s", resp.Status, body)
	}
	return nil
}

func handle(mux *http.ServeMux, path string, action func() error) {
	mux.HandleFunc("POST "+path, func(w http.ResponseWriter, _ *http.Request) {
		if err := action(); err != nil {
			log.Printf("%s: %v", path, err)
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func every(d time.Duration, f func()) {
	for range time.Tick(d) {
		f()
	}
}

func main() {
	log.SetPrefix("zstreamd: ")
	v := &vars{sent: map[string]string{}}
	vol := &volume{vars: v}
	if err := vol.poll(); err != nil {
		log.Fatal(err)
	}
	go every(pollEvery, func() {
		if err := vol.poll(); err != nil {
			log.Fatal(err)
		}
	})
	go every(resendEvery, func() {
		v.forget()
		vol.publish()
	})
	mux := http.NewServeMux()
	handle(mux, "/volume/up", func() error { return vol.step(1) })
	handle(mux, "/volume/down", func() error { return vol.step(-1) })
	handle(mux, "/volume/mute", vol.toggleMute)
	log.Printf("listening on %s", listenAddr)
	log.Fatal(http.ListenAndServe(listenAddr, mux))
}
