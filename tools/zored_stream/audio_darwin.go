package main

/*
#cgo LDFLAGS: -framework CoreAudio -framework AudioToolbox
#include <AudioToolbox/AudioServices.h>
#include <CoreAudio/CoreAudio.h>

static AudioObjectPropertyAddress volumeAddress = {kAudioHardwareServiceDeviceProperty_VirtualMainVolume, kAudioDevicePropertyScopeOutput, kAudioObjectPropertyElementMain};
static AudioObjectPropertyAddress muteAddress = {kAudioDevicePropertyMute, kAudioDevicePropertyScopeOutput, kAudioObjectPropertyElementMain};

static OSStatus outputDevice(AudioObjectID *device) {
	UInt32 size = sizeof *device;
	AudioObjectPropertyAddress address = {kAudioHardwarePropertyDefaultOutputDevice, kAudioObjectPropertyScopeGlobal, kAudioObjectPropertyElementMain};
	return AudioObjectGetPropertyData(kAudioObjectSystemObject, &address, 0, NULL, &size, device);
}

static OSStatus readAudio(Float32 *volume, UInt32 *mute) {
	AudioObjectID device;
	OSStatus status = outputDevice(&device);
	if (status != noErr) return status;
	UInt32 size = sizeof *volume;
	status = AudioObjectGetPropertyData(device, &volumeAddress, 0, NULL, &size, volume);
	if (status != noErr) return status;
	*mute = 0;
	if (!AudioObjectHasProperty(device, &muteAddress)) return noErr;
	size = sizeof *mute;
	return AudioObjectGetPropertyData(device, &muteAddress, 0, NULL, &size, mute);
}

static OSStatus writeVolume(Float32 volume) {
	AudioObjectID device;
	OSStatus status = outputDevice(&device);
	if (status != noErr) return status;
	return AudioObjectSetPropertyData(device, &volumeAddress, 0, NULL, sizeof volume, &volume);
}

static OSStatus writeMute(UInt32 mute) {
	AudioObjectID device;
	OSStatus status = outputDevice(&device);
	if (status != noErr) return status;
	if (!AudioObjectHasProperty(device, &muteAddress)) return kAudioHardwareUnknownPropertyError;
	return AudioObjectSetPropertyData(device, &muteAddress, 0, NULL, sizeof mute, &mute);
}
*/
import "C"

import (
	"fmt"
	"math"
)

func readAudio() (int, bool, error) {
	var volume C.Float32
	var mute C.UInt32
	if status := C.readAudio(&volume, &mute); status != 0 {
		return 0, false, fmt.Errorf("read audio: OSStatus %d", status)
	}
	return int(math.Round(float64(volume) * 100)), mute != 0, nil
}

func writeVolume(value int) error {
	if status := C.writeVolume(C.Float32(float32(value) / 100)); status != 0 {
		return fmt.Errorf("set volume: OSStatus %d", status)
	}
	return nil
}

func writeMute(value bool) error {
	mute := C.UInt32(0)
	if value {
		mute = 1
	}
	if status := C.writeMute(mute); status != 0 {
		return fmt.Errorf("set mute: OSStatus %d", status)
	}
	return nil
}
