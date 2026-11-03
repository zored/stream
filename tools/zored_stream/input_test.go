package main

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestInputSVG(t *testing.T) {
	for lang, want := range map[string]string{"en": "#012169", "ru": "#0039a6", "zh-Hans": ">zh<"} {
		raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(inputSVG(lang), "data:image/svg+xml;base64,"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), want) {
			t.Errorf("%s: %s", lang, raw)
		}
	}
}
