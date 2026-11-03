package main

/*
#cgo LDFLAGS: -framework IOBluetooth -framework Foundation -framework CoreAudio -Wl,-sectcreate,__TEXT,__info_plist,${SRCDIR}/Info.plist
#include <stdlib.h>
#include "headphones_darwin.h"
*/
import "C"

import (
	"fmt"
	"unsafe"
)

const maxAudioDevices = 64

func btConnected(address string) (bool, error) {
	a := C.CString(address)
	defer C.free(unsafe.Pointer(a))
	r := C.bt_connected(a)
	if r < 0 {
		return false, fmt.Errorf("bluetooth device %s not paired", address)
	}
	return r == 1, nil
}

func btConnect(address string) error {
	return btCall(address, "openConnection", func(a *C.char) C.int { return C.bt_connect(a) })
}

func btDisconnect(address string) error {
	return btCall(address, "closeConnection", func(a *C.char) C.int { return C.bt_disconnect(a) })
}

func btCall(address, name string, f func(*C.char) C.int) error {
	a := C.CString(address)
	defer C.free(unsafe.Pointer(a))
	if r := f(a); r != 0 {
		return fmt.Errorf("%s %s: 0x%x", name, address, uint32(r))
	}
	return nil
}

func audioDevices() ([]audioDevice, error) {
	var raw [maxAudioDevices]C.audio_device
	n := C.audio_devices(&raw[0], maxAudioDevices)
	if n < 0 {
		return nil, fmt.Errorf("list audio devices failed")
	}
	devices := make([]audioDevice, n)
	for i := range devices {
		t := uint32(raw[i].transport)
		devices[i] = audioDevice{
			id:        uint32(raw[i].id),
			name:      C.GoString(&raw[i].name[0]),
			transport: string([]byte{byte(t >> 24), byte(t >> 16), byte(t >> 8), byte(t)}),
			input:     raw[i].input != 0,
			output:    raw[i].output != 0,
		}
	}
	return devices, nil
}

func audioDefault(input bool) (uint32, error) {
	var id C.uint
	if status := C.audio_default(cBool(input), &id); status != 0 {
		return 0, fmt.Errorf("read default device: OSStatus %d", status)
	}
	return uint32(id), nil
}

func audioSetDefault(input bool, id uint32) error {
	if status := C.audio_set_default(cBool(input), C.uint(id)); status != 0 {
		return fmt.Errorf("set default device %d: OSStatus %d", id, status)
	}
	return nil
}

func cBool(b bool) C.int {
	if b {
		return 1
	}
	return 0
}
