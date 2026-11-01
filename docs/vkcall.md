# VK WorkSpace calls: microphone and camera

macOS only. Verified on VK WorkSpace Beta 27.1.0.144826 (calls SDK 26.2.0), macOS 26.

## Buttons

| Control | Action | Look |
|---|---|---|
| button 2 (`3/1`) | tap = toggle mic; hold ≥ 300 ms = push-to-talk (mic was on → mute while held) | LED |
| button 3 (`3/2`) | toggle camera | LED |
| tile `2/2` / `2/3` | same as button 2 / 3 | mic / camera icon |

| State | LED | Tile |
|---|---|---|
| `on` | green, dims for 1 s every 4 s | dark green, white icon |
| `off` | dim red | red, crossed icon |
| `unknown` | yellow, blinks every 2 s | amber, `?` |
| `none` (no call, client not running) | off | black |

Colours come from Companion custom variables `call_mic` and `call_cam`, never from a local toggle.

## Where the state lives

The call window is a separate Qt process, `vkws-call-service`, started with the client (before any call) as
`--rpc-port <port> --rpc-plaintext`. The port changes on every start. It is a QtGrpc server (h2c, no TLS, no auth, no reflection)
that the main app talks to; `vkcall` is just another client.

- Service `com.vk.calls.sdk.v1.CallAgent`, protobuf-lite: no descriptors in the binaries. Message and field names come from the
  QtProtobuf metadata strings in `VK WorkSpace` (`strings … | grep -A12 com.vk.calls.sdk.v1.<Message>`); field numbers follow the
  name order (checked on the fields `vkcall` uses, except `allCallsEnded` and `ended`: assumed).
- `Header` = `{1 timestamp (µs), 2 reqId (string), 3 callId}`, field 1 of every request. Without `reqId` requests fail with `errorCode 400`.
- `GetStatus(StatusRequest{hdr})` → `StatusReport`, `5 callIds` = active calls.
- `SubscribeToStatusNotifications(StatusRequest)` → stream of `StatusReport`: first message is a snapshot, then **deltas**
  (a message without `callIds` says nothing about calls; `6 allCallsEnded` = none left).
- `SubscribeToCallNotifications(CallInfoRequest{hdr.callId})` → stream of `CallEvent`: `4 state` → `CallState.2 media` →
  `MediaState{1 microOn, 2 cameraOn, 3 screencastOn}`; `8 ended`. First message is a snapshot; mouse clicks in the client
  arrive within a few ms. Several subscribers are fine: the client's own subscription keeps working.
- `ChangeMyMedia(ChangeMyMediaRequest{hdr, 2 microOn, 3 cameraOn})` → `GenericResponse{2 accepted, 3 errorCode}`. Sets **both**
  flags, so send the current value of the one you don't change. Camera start takes ~1 s.

Dead ends (verified):

- Hotkeys ⇧⌘A / ⇧⌘V (Settings → Call window) do nothing, not even typed by hand; posting them to the pid, via System Events or
  after focusing the window changes nothing.
- Accessibility: the call window exposes the labels ("Turn on microphone") as `AXStaticText` with only `AXRaise`, no buttons to press;
  synthetic clicks at their position don't toggle.
- CoreAudio: the mic device keeps running while muted in the client. CoreMediaIO `DeviceIsRunningSomewhere` does follow the camera.
- `logs/call-service.log` has `MICROPHONE is ON/OFF`, `CAMERA is ON/OFF`, call start/end lines — usable for debugging, not as an API.

## vkcall

`tools/vkcall/main.go`, `make vkcall` → `~/.local/bin/vkcall` + LaunchAgent `com.zored.stream.vkcall`.
Log: `~/Library/Logs/zored-stream/vkcall.log`. `make vkcall-uninstall` removes it.

- HTTP on `127.0.0.1:47114`: `POST /mic/down`, `/mic/up`, `/cam/down`; `409` when there is no call with a known state.
  Push-to-talk timing lives in the daemon (down/up arrival times).
- Finds the process with `sysctl` (`kern.proc.all`, `kern.procargs2`) every 2 s while disconnected; while connected everything
  is stream-driven, no polling. Variables are pushed on change and re-sent every 10 s (heals a Companion restart).
- A call that appears while the daemon runs is muted (mic and camera) if the client joined it with either on.
- `unknown`: stream error while the client runs, unparsable message, rejected `ChangeMyMedia`, or no confirming event within 2 s
  (then it resubscribes and takes the snapshot).
- A client update can rename or renumber fields; the daemon then logs the error and the buttons go yellow.

Debug by hand (Python with `grpcio` + `protoc --decode_raw`): build the raw request bytes and call
`/com.vk.calls.sdk.v1.CallAgent/<Method>` with identity serializers.
