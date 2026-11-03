package main

import "testing"

var (
	macOut   = audioDevice{id: 1, name: "Динамики MacBook Pro", transport: "bltn", output: true}
	macIn    = audioDevice{id: 2, name: "Микрофон MacBook Pro", transport: "bltn", input: true}
	logi     = audioDevice{id: 3, name: "Logi Z407", transport: "usb ", output: true}
	camera   = audioDevice{id: 4, name: "USB Camera", transport: "usb ", input: true}
	accentum = audioDevice{id: 5, name: "ACCENTUM", transport: "blue", input: true, output: true}
)

func TestSpeakersRoute(t *testing.T) {
	for _, c := range []struct {
		devices []audioDevice
		out, in uint32
	}{
		{[]audioDevice{macOut, macIn, accentum}, 1, 2},
		{[]audioDevice{macOut, macIn, logi, camera, accentum}, 3, 4},
	} {
		r, err := speakersRoute(c.devices)
		if err != nil || r.output.id != c.out || r.input.id != c.in {
			t.Errorf("%v: got %d/%d %v", c.devices, r.output.id, r.input.id, err)
		}
	}
}

func TestHeadphonesRoute(t *testing.T) {
	r, err := headphonesRoute([]audioDevice{macOut, macIn, accentum})
	if err != nil || r.output.id != 5 || r.input.id != 5 {
		t.Fatalf("got %v %v", r, err)
	}
	if _, err := headphonesRoute([]audioDevice{macOut, macIn}); err == nil {
		t.Fatal("want error without headphones")
	}
}

func TestSpeakersArrived(t *testing.T) {
	before := deviceKeys([]audioDevice{macOut, macIn})
	if !speakersArrived(before, []audioDevice{macOut, macIn, logi, camera}) {
		t.Error("want arrival on KVM switch")
	}
	if speakersArrived(deviceKeys([]audioDevice{macOut, macIn, logi, camera}), []audioDevice{macOut, macIn, logi, camera}) {
		t.Error("want no arrival when nothing changed")
	}
	if speakersArrived(before, []audioDevice{macOut, macIn, accentum}) {
		t.Error("want no arrival for headphones")
	}
}
