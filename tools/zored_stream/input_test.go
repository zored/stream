package main

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestInputSVG(t *testing.T) {
	for lang, want := range map[string]string{"en": "#012169", "ru": "#0039a6", "zh-Hans": ">zh<"} {
		raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(inputSVG(inputCode(lang), inputCode(lang), 1), "data:image/svg+xml;base64,"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), want) {
			t.Errorf("%s: %s", lang, raw)
		}
	}
}

func TestInputSVGFade(t *testing.T) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(inputSVG("en", "ru", 0.25), "data:image/svg+xml;base64,"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`opacity="0.75"`, `opacity="0.25"`, "#012169", "#0039a6"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("missing %s: %s", want, raw)
		}
	}
}
