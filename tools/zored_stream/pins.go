package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
)

type agentPins struct {
	file string
	set  map[string]bool
}

func loadPins(file string) (*agentPins, error) {
	p := &agentPins{file: file, set: map[string]bool{}}
	b, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return nil, err
	}
	var hosts []string
	if err := json.Unmarshal(b, &hosts); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	for _, h := range hosts {
		p.set[h] = true
	}
	return p, nil
}

func (p *agentPins) has(host string) bool {
	return host != "" && p.set[host]
}

func (p *agentPins) toggle(host string) (bool, error) {
	on := !p.set[host]
	if on {
		p.set[host] = true
	} else {
		delete(p.set, host)
	}
	if err := p.save(); err != nil {
		if on {
			delete(p.set, host)
		} else {
			p.set[host] = true
		}
		return !on, err
	}
	return on, nil
}

func (p *agentPins) list() []string {
	hosts := make([]string, 0, len(p.set))
	for h := range p.set {
		hosts = append(hosts, h)
	}
	slices.Sort(hosts)
	return hosts
}

func (p *agentPins) save() error {
	b, err := json.Marshal(p.list())
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p.file), 0o755); err != nil {
		return err
	}
	tmp := p.file + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p.file)
}
