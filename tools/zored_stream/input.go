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

func inputSVG(lang string) string {
	code, _, _ := strings.Cut(strings.ToLower(lang), "-")
	return dataURI(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">`+
		`<text x="50" y="50" dy="0.35em" text-anchor="middle" font-family="JetBrains Mono" font-size="44" fill="#fff">%s</text></svg>`,
		html.EscapeString(code)))
}
