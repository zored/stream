package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestVoipState(t *testing.T) {
	lines := []string{
		`I [05 12:14:55.156] [#callstat] (call_stat_impl.cc:184): VM_StartOutgoing: id:733aeb21-2676-4583-860f-75cb3495081b is_video:0 participants: app_data:{ "call_type" : "link", "url" : "4527e2f0964d4f648a9c41b6b36dcf16" }`,
		`I [05 12:14:55.156] [#voip3sdk_w] (MultimediaStateModel.cc:269): CallObserversWithCurrentState: _speaker_state:0, _audio_playback_mute:0, _media_audio_enable_tx:1, _media_video_enable_tx:0`,
		`I [05 12:19:30.315] [#voip3sdk_w] (VoipController.cc:905): OnStateMediaAudioTXEnabled: call_id: 733aeb21-2676-4583-860f-75cb3495081b enabled=0`,
		`I [05 12:19:31.315] [#voip3sdk_w] (VoipController.cc:912): OnStateMediaVideoTXEnabled: call_id: 733aeb21-2676-4583-860f-75cb3495081b enabled=1`,
		`I [05 12:19:32.000] [#voip3sdk_w] (VoipController.cc:378): Action_EnableMicrophone: call_id: 733aeb21-2676-4583-860f-75cb3495081b enabled=1`,
	}
	var st voipState
	for _, l := range lines {
		if e, ok := parseVoipLine(l); ok {
			st.apply(e)
		}
	}
	want := voipState{callID: "733aeb21-2676-4583-860f-75cb3495081b", media: [2]bool{false, true}, known: [2]bool{true, true}, started: true}
	if st != want {
		t.Fatalf("got %+v, want %+v", st, want)
	}
	for _, l := range []string{
		`I [05 12:29:51.546] [#proxy] (VoipEngineObserverProxy.cc:178): ->Observer::OnCallTerminated: id:67f9806c-d171-4dc5-8e1a-5871380bef8c reason:local:TR_H`,
		`I [05 12:29:51.546] [#voip3sdk_w] (VoipController.cc:674): OnCallTerminated: terminated_locally=1 reason=0`,
	} {
		if e, ok := parseVoipLine(l); ok {
			st.apply(e)
		}
	}
	if st != (voipState{}) {
		t.Fatalf("after end: %+v", st)
	}
}

func TestVoipStateMediaBeforeStart(t *testing.T) {
	var st voipState
	for _, l := range []string{
		`I [05 12:54:20.711] [#voip3sdk_w] (VoipController.cc:905): OnStateMediaAudioTXEnabled: call_id: c2ff361b-10bf-491e-ba48-91a055189174 enabled=1`,
		`I [05 12:54:20.711] [#voip3sdk_w] (VoipController.cc:912): OnStateMediaVideoTXEnabled: call_id: c2ff361b-10bf-491e-ba48-91a055189174 enabled=0`,
		`I [05 12:54:20.711] [#callstat] (call_stat_impl.cc:184): VM_StartOutgoing: id:c2ff361b-10bf-491e-ba48-91a055189174 is_video:0 participants:`,
		`I [05 12:54:20.716] [#voip3sdk_w] (VoipController.cc:905): OnStateMediaAudioTXEnabled: call_id: c2ff361b-10bf-491e-ba48-91a055189174 enabled=0`,
	} {
		if e, ok := parseVoipLine(l); ok {
			st.apply(e)
		}
	}
	want := voipState{callID: "c2ff361b-10bf-491e-ba48-91a055189174", known: [2]bool{true, true}, started: true}
	if st != want {
		t.Fatalf("got %+v, want %+v", st, want)
	}
}

func TestVoipStateRotated(t *testing.T) {
	var st voipState
	e, _ := parseVoipLine(`I [05 12:26:16.272] [#voip3sdk_w] (VoipController.cc:905): OnStateMediaAudioTXEnabled: call_id: 67f9806c-d171-4dc5-8e1a-5871380bef8c enabled=1`)
	st.apply(e)
	if st.callID != "67f9806c-d171-4dc5-8e1a-5871380bef8c" || st.known != [2]bool{true, false} || !st.media[0] {
		t.Fatalf("got %+v", st)
	}
}

func TestVoipNewest(t *testing.T) {
	dir := t.TempDir()
	if got, err := voipNewest(dir); err != nil || got != "" {
		t.Fatalf("empty: %q %v", got, err)
	}
	for _, n := range []string{"voip_2026.11.06_16.00.29.log", "voip_2026.11.06_19.10.03.log", "voip_2026.10.30_23.59.59.log", "main.log"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := voipNewest(dir)
	if err != nil || got != filepath.Join(dir, "voip_2026.11.06_19.10.03.log") {
		t.Fatalf("got %q %v", got, err)
	}
}

func TestVoipMicTapAndHold(t *testing.T) {
	for _, tc := range []struct {
		name      string
		keyTakes  time.Duration
		releaseIn time.Duration
		wantPress int
		wantMicOn bool
	}{
		{"tap while key is slow", holdMin + 200*time.Millisecond, 50 * time.Millisecond, 1, true},
		{"hold", 10 * time.Millisecond, holdMin + 100*time.Millisecond, 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var presses []int
			orig := voipPress
			voipPress = func(key int) error {
				presses = append(presses, key)
				time.Sleep(tc.keyTakes)
				return nil
			}
			defer func() { voipPress = orig }()
			d := &call{changed: make(chan struct{}, 1), voipID: "x", callID: "x", mic: off, cam: off}
			down := make(chan error)
			go func() { down <- d.micDown() }()
			time.Sleep(tc.releaseIn)
			if err := d.micUp(); err != nil {
				t.Fatal(err)
			}
			if err := <-down; err != nil {
				t.Fatal(err)
			}
			d.mu.Lock()
			defer d.mu.Unlock()
			d.stopConfirmLocked()
			if len(presses) != tc.wantPress || d.want[0] != tc.wantMicOn {
				t.Fatalf("presses %v, want mic %v", presses, d.want)
			}
		})
	}
}
