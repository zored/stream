# VK WorkSpace calls: microphone and camera

macOS only.

## Supported versions

| Component | Version | Used for |
|---|---|---|
| VK WorkSpace Beta | 27.1.0.144826 | host app, macOS 26 |
| calls SDK (`vkws-call-service`) | 26.2.0 | new engine: gRPC, below |
| voip3sdk (in the main process) | 26.1.10.3774 | old engine: calls joined by link, see [Old engine](#old-engine-voip3sdk) |

The client picks the engine per call: a call joined by link (`"call_type":"link"`) runs on voip3sdk and never reaches
`vkws-call-service`. Another version may rename log lines or fields: the buttons go yellow (`unknown`) or stay `none`.

## Buttons

| Control | Action | Look |
|---|---|---|
| button 3 (`3/2`) | in a call: tap = toggle mic, hold ≥ 300 ms = push-to-talk; free outside a call | LED |
| button 4 (`3/3`) | toggle camera | LED |
| button 5 (`3/4`), tile `2/4` | hang up in a call (button 5: calendar outside a call) | white LED / red icon on black while in a call |
| tile `2/2` / `2/3` | same as button 3 / 4 in a call (buttons sit under the tiles) | mic / camera icon |

| State | LED | Tile |
|---|---|---|
| `on` | red, dims for 1 s every 4 s | red, white icon |
| `off` | dim white | black, white crossed icon |
| `pending` (asked, not confirmed yet: camera takes seconds) | yellow | amber, `…` |
| `unknown` | yellow, blinks every 2 s | amber, `?` |
| `none` (no call, client not running) | off | black |

Colours come from Companion custom variables `call_mic` and `call_cam`, never from a local toggle.

## Where the state lives

The call window is a separate Qt process, `vkws-call-service`, started with the client (before any call) as
`--rpc-port <port> --rpc-plaintext`. The port changes on every start. It is a QtGrpc server (h2c, no TLS, no auth, no reflection)
that the main app talks to; `zored_stream` is just another client.

- Service `com.vk.calls.sdk.v1.CallAgent`, protobuf-lite: no descriptors in the binaries. Message and field names come from the
  QtProtobuf metadata strings in `VK WorkSpace` (`strings … | grep -A12 com.vk.calls.sdk.v1.<Message>`); field numbers follow the
  name order (checked on the fields `zored_stream` uses, except `allCallsEnded` and `ended`: assumed).
- `Header` = `{1 timestamp (µs), 2 reqId (string), 3 callId}`, field 1 of every request. Without `reqId` requests fail with `errorCode 400`.
- `GetStatus(StatusRequest{hdr})` → `StatusReport`, `5 callIds` = active calls.
- `SubscribeToStatusNotifications(StatusRequest)` → stream of `StatusReport`: first message is a snapshot, then **deltas**
  (a message without `callIds` says nothing about calls; `6 allCallsEnded` = none left).
- `SubscribeToCallNotifications(CallInfoRequest{hdr.callId})` → stream of `CallEvent`: `4 state` → `CallState.2 media` →
  `MediaState{1 microOn, 2 cameraOn, 3 screencastOn}`; `8 ended`. First message is a snapshot; mouse clicks in the client
  arrive within a few ms. Several subscribers are fine: the client's own subscription keeps working.
- `ChangeMyMedia(ChangeMyMediaRequest{hdr, 2 microOn, 3 cameraOn})` → `GenericResponse{2 accepted, 3 errorCode}`. Sets **both**
  flags, so send the current value of the one you don't change. Camera start takes ~1–3 s.
- `EndCall(EndCallRequest{hdr.callId})` → `GenericResponse`. The call is addressed by `hdr.callId` or by `2 origReqId` (the client's
  call request number, `CallEvent.2`), not both: both → `valid: 0`, `errorCode 400`.

Dead ends (verified):

- Hotkeys ⇧⌘A / ⇧⌘V (Settings → Call window) do nothing, not even typed by hand; posting them to the pid, via System Events or
  after focusing the window changes nothing.
- Accessibility: the call window exposes the labels ("Turn on microphone") as `AXStaticText` with only `AXRaise`, no buttons to press;
  synthetic clicks at their position don't toggle.
- CoreAudio: the mic device keeps running while muted in the client. CoreMediaIO `DeviceIsRunningSomewhere` does follow the camera.
- `logs/call-service.log` has `MICROPHONE is ON/OFF`, `CAMERA is ON/OFF`, call start/end lines — usable for debugging, not as an API.

## zored_stream call module

`tools/zored_stream/call.go` in the [one daemon](daemon.md) (was the separate `vkcall`), `make zored_stream`.
Log: `~/Library/Logs/zored-stream/zored_stream.log`.

- HTTP on `127.0.0.1:47114`: `POST /mic/down`, `/mic/up`, `/cam/down`, `/call/end`; `409` when there is no call with a known state.
  Push-to-talk timing lives in the daemon (down/up arrival times).
- Finds the process with `sysctl` (`kern.proc.all`, `kern.procargs2`) every 2 s while disconnected; while connected everything
  is stream-driven, no polling. Variables are pushed on change and re-sent every 10 s (heals a Companion restart).
- A call that appears while the daemon runs is muted (mic and camera) if the client joined it with either on.
- `pending`: a flag asked to change until the client's event confirms it.
- `unknown`: stream error while the client runs, unparsable message, rejected `ChangeMyMedia`, or no confirming event within 5 s
  (then it resubscribes and takes the snapshot).
- A client update can rename or renumber fields; the daemon then logs the error and the buttons go yellow.

Debug by hand (Python with `grpcio` + `protoc --decode_raw`): build the raw request bytes and call
`/com.vk.calls.sdk.v1.CallAgent/<Method>` with identity serializers.

## Old engine (voip3sdk)

Calls joined by link run inside the main `VK WorkSpace` process: no gRPC, no port, the call window (`In call: <n>`, n = people
with self) is QML with no accessibility inside. `voip.go` / `voip_darwin.go` in `zored_stream`, verified on a live 18-person call:

- State: tail of the newest `logs/voip_<date>_<time>.log` (new file per call and hourly), polled every 150 ms; on start the
  files of the last 12 h are replayed.
  - start: `VM_StartOutgoing: id:<callId> … "url" : "<meetingId>"`; initial media: `CallObserversWithCurrentState: … _media_audio_enable_tx:1, _media_video_enable_tx:0`
  - media: `OnStateMediaAudioTXEnabled: call_id: <id> enabled=0|1`, same with `Video`; mouse clicks in the client log the same lines
  - end: `(VoipController.cc:…): OnCallTerminated:`; also `none` when the process is gone
  - participants (used by vkws-party, not by buttons): `AdhocCallSession.cc:659 FocusCallSession_StateUpdated … [email|JOINED|aVC|WEBRTC_CLIENT]…`,
    full list without self, `JOINING`/`JOINED`; `A`/`V` uppercase = audio/video on (assumed)
- Mic: ⇧⌘A posted to the pid (`CGEventPostToPid`, no focus needed). Camera: ⇧⌘V. Both are
  **toggles**: the daemon presses only when the wanted value differs from the log state (or the pending one).
- Hang up: press the `In call` window's AX close button (returns `-25204` but opens «End call — Are you sure?»), wait 0.8 s,
  keypad Enter (keycode 76) to the pid confirms Yes. No focus needed. Return (36) does not confirm it, ⌘W only sometimes opens it.
- Focus: a hotkey posted to the pid is sometimes ignored until the call window is key. Each press activates VK WorkSpace, raises the `In call` window (AXMain + AXFocused), sends the key and reactivates the previous front app (~0.3 s flicker). Hang up: icons go `pending` at once, the dialog appears ~1 s after close, a full hang up takes ~2 s.
- Dead ends: Return / mouse clicks posted to the pid on the dialog, no hang-up shortcut in the binary, DevTools on `127.0.0.1:3333` lists web tabs only.
