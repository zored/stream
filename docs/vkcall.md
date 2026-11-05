# VK WorkSpace calls: microphone and camera

macOS only. Verified on VK WorkSpace Beta 27.1.0.144826 (calls SDK 26.2.0), macOS 26.

## Buttons

| Control | Action | Look |
|---|---|---|
| button 3 (`3/2`) | in a call (layer `button_3` = `call`): tap = toggle mic; hold ≥ 300 ms = push-to-talk (mic was on → mute while held); outside a call the unread chats | LED |
| button 4 (`3/3`) | toggle camera | LED |
| button 5 (`3/4`), tile `2/4` | hang up in a call (button 5: calendar outside a call) | white LED / red icon on black while in a call |
| tile `2/2` / `2/3` | same as button 3 / 4 (buttons sit under the tiles) | mic / camera icon |

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
