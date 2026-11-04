package main

import (
	"embed"
	"fmt"
	"strings"
)

const (
	iconSize     = 56.0
	iconCY       = 50.0
	iconCaptionY = 40.0
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

func iconAt(name, color string, cx, cy, size float64) string {
	return fmt.Sprintf(`<g transform="translate(%g %g) scale(%g)" fill="%s">%s</g>`,
		cx-size/2, cy-size/2, size/16, color, iconBody(name))
}
