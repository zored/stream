package main

import "log"

func publishStripFlip(v *vars) {
	o, err := readOrientation()
	if err != nil {
		log.Print(err)
		return
	}
	flip := "0"
	if o == 90 || o == 180 {
		flip = "1"
	}
	v.set("strip_flip", flip)
}
