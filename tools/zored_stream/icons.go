package main

import (
	"embed"
	"fmt"
	"html"
	"strings"
)

const (
	iconSize     = 56.0
	iconCY       = 38.0
	captionY     = 89.0
	captionSize  = 16.0
	captionRunes = 10
)

//go:embed icons/*.svg
var iconFiles embed.FS

func iconBody(name string) string {
	data, err := iconFiles.ReadFile("icons/" + name + ".svg")
	if err != nil {
		panic(err)
	}
	s := string(data)
	start, end := strings.Index(s, ">")+1, strings.LastIndex(s, "</svg>")
	if start <= 0 || end < start {
		panic("icon " + name + ": not an svg")
	}
	return strings.TrimSpace(s[start:end])
}

func tileIcon(name, color string) string {
	return dataURI(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">` + iconAt(name, color, 50, iconCY, iconSize) + `</svg>`)
}

func captionTile(name, color, caption, font string) string {
	if caption == "" {
		return tileIcon(name, color)
	}
	if font == "" {
		font = stripFont
	}
	return dataURI(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">` + iconAt(name, color, 50, iconCY, iconSize) +
		captionText(caption, font) + `</svg>`)
}

func captionText(caption, font string) string {
	if r := []rune(caption); len(r) > captionRunes {
		caption = string(r[:captionRunes])
	}
	return fmt.Sprintf(`<text x="50" y="%g" text-anchor="middle" font-family="%s" font-size="%g" fill="#fff">%s</text>`,
		captionY, font, captionSize, html.EscapeString(caption))
}

func iconAt(name, color string, cx, cy, size float64) string {
	return fmt.Sprintf(`<g transform="translate(%g %g) scale(%g)" fill="%s">%s</g>`,
		cx-size/2, cy-size/2, size/16, color, iconBody(name))
}
