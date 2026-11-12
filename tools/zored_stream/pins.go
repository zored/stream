package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

const pinPendingFor = 30 * time.Second

type pinWant struct {
	on bool
	at time.Time
}

type agentPins struct {
	file    string
	mod     time.Time
	starred map[string]bool
	pending map[string]pinWant
}

func newPins(desktopConfig string) *agentPins {
	return &agentPins{file: desktopConfig, starred: map[string]bool{}, pending: map[string]pinWant{}}
}

func (p *agentPins) refresh() error {
	st, err := os.Stat(p.file)
	if err != nil {
		return err
	}
	if !st.ModTime().Equal(p.mod) {
		b, err := os.ReadFile(p.file)
		if err != nil {
			return err
		}
		var cfg struct {
			Preferences struct {
				EpitaxyPrefs struct {
					Starred []string `json:"starred-local-code-sessions"`
				} `json:"epitaxyPrefs"`
			} `json:"preferences"`
		}
		if err := json.Unmarshal(b, &cfg); err != nil {
			return fmt.Errorf("%s: %w", p.file, err)
		}
		p.mod = st.ModTime()
		clear(p.starred)
		for _, h := range cfg.Preferences.EpitaxyPrefs.Starred {
			p.starred[h] = true
		}
	}
	for h, w := range p.pending {
		if p.starred[h] == w.on || time.Since(w.at) > pinPendingFor {
			delete(p.pending, h)
		}
	}
	return nil
}

func (p *agentPins) has(host string) bool {
	if w, ok := p.pending[host]; ok {
		return w.on
	}
	return p.starred[host]
}

func (p *agentPins) toggle(host string) bool {
	on := !p.has(host)
	p.pending[host] = pinWant{on, time.Now()}
	return on
}
