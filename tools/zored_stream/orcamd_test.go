package main

import "testing"

func TestOrcaMarkdown(t *testing.T) {
	for in, want := range map[string]string{
		"## Итог\n\nготово **точно**, `make test` ok":       "\x01Итог\x01\nготово \x01точно\x01, \x02make test\x02 ok",
		"- one\n  - two\n* [x] done":                        "• one\n • two\n• done",
		"| a | b |\n|---|:-:|\n| **1** | 2 |":              "a · b\n\x011\x01 · 2",
		"```go\nx := 1\n```\n---\n> note":                   "\x02x := 1\x02\n│ note",
		"see [docs](http://x) and *em* snake_case a * b":    "see docs and em snake_case a * b",
	} {
		if got := orcaMarkdown(in); got != want {
			t.Errorf("orcaMarkdown(%q) = %q, want %q", in, got, want)
		}
	}
}
