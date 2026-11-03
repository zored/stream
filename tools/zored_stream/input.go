package main

import (
	"fmt"
	"html"
	"log"
	"strings"
)

type input struct {
	vars *vars
}

func (in *input) publish() {
	lang, err := inputCurrent()
	if err != nil {
		log.Print(err)
		return
	}
	in.vars.set("input_svg", inputSVG(lang))
}

var inputFlags = map[string]string{
	"ru": `<rect width="60" height="13.33" fill="#fff"/><rect y="13.33" width="60" height="13.34" fill="#0039a6"/><rect y="26.67" width="60" height="13.33" fill="#d52b1e"/>`,
	"en": `<rect width="60" height="40" fill="#012169"/><path d="M0 0L60 40M60 0L0 40" stroke="#fff" stroke-width="8"/>` +
		`<path d="M0 0L60 40M60 0L0 40" stroke="#c8102e" stroke-width="3"/>` +
		`<path d="M30 0V40M0 20H60" stroke="#fff" stroke-width="12"/><path d="M30 0V40M0 20H60" stroke="#c8102e" stroke-width="7"/>`,
}

func inputSVG(lang string) string {
	code, _, _ := strings.Cut(strings.ToLower(lang), "-")
	body, ok := inputFlags[code]
	if !ok {
		return dataURI(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">`+
			`<text x="50" y="50" dy="0.35em" text-anchor="middle" font-family="JetBrains Mono" font-size="44" fill="#fff">%s</text></svg>`,
			html.EscapeString(code)))
	}
	return dataURI(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><clipPath id="c"><rect y="20" width="100" height="60" rx="4"/></clipPath>` +
		`<g clip-path="url(#c)"><g transform="translate(0 20) scale(1.6667 1.5)">` + body + `</g></g></svg>`)
}
