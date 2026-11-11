package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func readOrientation() (int, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return 0, err
	}
	path := filepath.Join(home, ".config", "zored-stream", "orientation")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 90, nil
	}
	if err != nil {
		return 0, err
	}
	text := strings.TrimSpace(string(data))
	o, err := strconv.Atoi(text)
	if err != nil || o < 0 || o > 270 || o%90 != 0 {
		return 0, fmt.Errorf("bad orientation %q in %s", text, path)
	}
	return o, nil
}

func stripFlipped(o int, right bool) bool {
	return o == 180 || (o == 90 && !right) || (o == 270 && right)
}

func upright(o int, right bool) float64 {
	turn := o
	if stripFlipped(o, right) {
		turn += 180
	}
	return float64((360 - turn%360) % 360)
}
