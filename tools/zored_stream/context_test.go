package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContextOf(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".claude", "projects", "-x")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "abc.jsonl")
	lines := `{"message":{"model":"claude-opus-5-5","usage":{"input_tokens":1,"cache_read_input_tokens":99,"cache_creation_input_tokens":0}}}
{"message":{"model":"claude-opus-5-5","usage":{"input_tokens":2,"cache_read_input_tokens":100000,"cache_creation_input_tokens":2638}}}
{"message":{"model":"<synthetic>","usage":{"input_tokens":0}}}
{"type":"user"}
`
	if err := os.WriteFile(path, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	c := newContexts(home)
	ctx, err := c.of("abc")
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Tokens != 102640 || ctx.Window != contextWindow || ctx.Percent < 10.26 || ctx.Percent > 10.27 {
		t.Errorf("got %+v", ctx)
	}
	if _, err := c.of("missing"); err == nil {
		t.Error("missing transcript: no error")
	}
	if contextWindowFor("claude-haiku-5-5") != contextWindowLow {
		t.Error("haiku window")
	}
}

func TestAgentsContextBar(t *testing.T) {
	ctx := &chatContext{Percent: 92}
	svg := agentsSVG(nil, nil, -1, -1, 1, ctx)
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(svg, "data:image/svg+xml;base64,"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `<rect width="92.0" height="4" fill="#FF6666"/>`) {
		t.Errorf("no red context bar in %s", raw)
	}
}
