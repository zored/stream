package main

/*
#cgo LDFLAGS: -framework CoreAudio -framework AudioToolbox
#include "audio_darwin.h"
*/
import "C"

import (
	"fmt"
	"math"
)

var audioChanged func()

//export goAudioChanged
func goAudioChanged() {
	audioChanged()
}

func watchAudio(onChange func()) error {
	audioChanged = onChange
	if status := C.audio_watch(); status != 0 {
		return fmt.Errorf("watch audio: OSStatus %d", status)
	}
	return nil
}

func readAudio() (int, bool, error) {
	var volume C.Float32
	var mute C.UInt32
	if status := C.audio_read(&volume, &mute); status != 0 {
		return 0, false, fmt.Errorf("read audio: OSStatus %d", status)
	}
	return int(math.Round(float64(volume) * 100)), mute != 0, nil
}

func writeVolume(value int) error {
	if status := C.audio_write_volume(C.Float32(float32(value) / 100)); status != 0 {
		return fmt.Errorf("set volume: OSStatus %d", status)
	}
	return nil
}

func writeMute(value bool) error {
	mute := C.UInt32(0)
	if value {
		mute = 1
	}
	if status := C.audio_write_mute(mute); status != 0 {
		return fmt.Errorf("set mute: OSStatus %d", status)
	}
	return nil
}
