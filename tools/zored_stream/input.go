package main

import (
	"fmt"
	"html"
	"log"
	"strings"
	"sync"
	"time"
)

const (
	inputFadeFor    = 300 * time.Millisecond
	inputFadeFrames = 12
)

type input struct {
	mu    sync.Mutex
	vars  *vars
	shown string
	gen   int
}

func (in *input) publish() {
	lang, err := inputCurrent()
	if err != nil {
		log.Print(err)
		return
	}
	code := inputCode(lang)
	in.mu.Lock()
	from := in.shown
	in.shown = code
	in.gen++
	gen := in.gen
	in.mu.Unlock()
	if from == "" || from == code {
		in.vars.set("input_svg", inputSVG(code, code, 1))
		return
	}
	go in.fade(from, code, gen)
}

func (in *input) fade(from, to string, gen int) {
	for i := 1; i <= inputFadeFrames; i++ {
		in.mu.Lock()
		stale := in.gen != gen
		in.mu.Unlock()
		if stale {
			return
		}
		in.vars.set("input_svg", inputSVG(from, to, float64(i)/inputFadeFrames))
		time.Sleep(inputFadeFor / inputFadeFrames)
	}
}

var inputFlags = map[string]string{
	"ru": `<rect width="60" height="13.33" fill="#fff"/><rect y="13.33" width="60" height="13.34" fill="#0039a6"/><rect y="26.67" width="60" height="13.33" fill="#d52b1e"/>`,
	"en": `<rect width="60" height="40" fill="#012169"/><path d="M0 0L60 40M60 0L0 40" stroke="#fff" stroke-width="8"/>` +
		`<path d="M0 0L60 40M60 0L0 40" stroke="#c8102e" stroke-width="3"/>` +
		`<path d="M30 0V40M0 20H60" stroke="#fff" stroke-width="12"/><path d="M30 0V40M0 20H60" stroke="#c8102e" stroke-width="7"/>`,
}

func inputCode(lang string) string {
	code, _, _ := strings.Cut(strings.ToLower(lang), "-")
	return code
}

func inputLayer(code string, opacity float64) string {
	if body, ok := inputFlags[code]; ok {
		return fmt.Sprintf(`<g opacity="%.2f" clip-path="url(#c)"><g transform="translate(20 30)">%s</g></g>`, opacity, body)
	}
	return fmt.Sprintf(`<text opacity="%.2f" x="50" y="50" dy="0.35em" text-anchor="middle" font-family="JetBrains Mono" font-size="32" fill="#fff">%s</text>`,
		opacity, html.EscapeString(code))
}

func inputSVG(from, to string, t float64) string {
	layers := inputLayer(to, t)
	if from != to && t < 1 {
		layers = inputLayer(from, 1-t) + layers
	}
	return dataURI(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><clipPath id="c"><rect x="20" y="30" width="60" height="40" rx="3"/></clipPath>` +
		layers + `</svg>`)
}
