# macOS helpers

The `zored_stream` daemon (HTTP) backs the Companion pages.

## Media keys — play / next / prev

`POST :47113/key/<play|next|prev|volup|voldown|mute>` in `zored_stream` (`mediakey*.go`, `mediakey_darwin.m`); unknown name → 404.
Replaced the Swift CLI `mediakey` (Companion `exec`, deleted 2026-11-03).

- Posts system-defined media key events (`NSEvent` subtype 8 → `CGEvent.post(.cghidEventTap)`): reaches whatever owns
  Now Playing, browser tabs included, like a real media key.
- Needs Accessibility for the signed daemon (A11); without it the route answers 409 and the log says so.
  Verified 2026-11-03: `POST /api/location/1/1/0/press` toggles Yandex Music play/pause.
- Gotcha (old CLI): a process exiting right after posting drops the events silently; the daemon lives on, so it doesn't.
- Dead end: private `MRMediaRemoteSendCommand` returns true but browsers ignore it.

## zored_stream — volume and Now Playing

Modules of the [one daemon](daemon.md) (`tools/zored_stream/`, was the separate `streamd`). `make zored_stream` builds it,
installs the LaunchAgent `com.zored.stream.zored_stream` and (re)starts it with `launchctl kickstart -k`
(a `bootout` + `bootstrap` pair races). Log: `~/Library/Logs/zored-stream/zored_stream.log`.

- HTTP on `127.0.0.1:47113`: `POST /volume/up`, `/volume/down` (acceleration 1 → 4 % per detent by tick interval),
  `/volume/mute`, `/volume/drag?dy=<px>` (0.4 %/px). Companion knobs call it with `curl -fsS -m 1 -X POST …`.
- Volume via CoreAudio (`kAudioHardwareServiceDeviceProperty_VirtualMainVolume`, same scale as the system slider). No native HUD:
  the strip is the HUD.
- Pushes Companion custom variables (must exist: `make deploy` creates them from `companion/custom-variables.json`):
  - `volume` (text, `MUTE` when muted; not shown on the strip, the white bar is the HUD)
  - the left-strip bar, shared with brightness (`bar.go`): `bar_value` (eased at 60 fps), `bar_alpha` (0–100, fade in fast, out slow; any change incl. keyboard + 1 s), `bar_mode` (`volume`, `mac`, `ctl`, `seek`: who moved it last; a hidden bar jumps to the new mode's value instead of easing from the old one)
  - `np_playing` (`1`/`0`), `np_title`, `np_artist`, `np_track` (`artist — title`), `np_app` (bundle id of the Now Playing app),
    `np_svg` (base64 SVG data URI for the track strip: like mark (a heart as wide as the strip, 6% inset), lowercase gray artist `|` bold title, each cut to 30 characters with `…`, split into two lines when one line would be tiny; the heart is an SVG path (skia has no emoji fallback), reset on track change; an empty title is held back for 3 s; an image layer can't take raw SVG,
    Companion cuts data at the first `=`),
    polled every 0.5 s by the perl child (below), sent to the daemon as JSON lines on change; `publish` is serialized (`publishMu`): the 3 s hold timer and a new track used to interleave their writes and leave the old strip until the 10 s resend *(assumed cause of a stale track; natural transitions ×3 and next ×1 measured < 1 s, 2026-11-04)*
  - `strip_flip` (`1` at orientation 90°/180° = the fork turns the left strip 180°, else `0`): `stripflip.go` reads `~/.config/zored-stream/orientation` on every `bar` / progress publish; the left-strip bars (`volume_cell`) mirror their fill by it so they still grow bottom-up on the device. The AI bar on the right strip is not mirrored.
  - `np_progress` (0–100) — playback position, drawn bottom-up in `np_tint` behind the track text on the left strip.
    The perl child sends `duration`, `elapsed`, `rate`, `at` (unix time of `elapsed`) from MediaRemote; the daemon extrapolates
    `elapsed + (now − at) × rate` every 3 s, so no extra polling; paused or no duration → the value doesn't change, nothing is sent.
  - `np_tint` (`tint.go`) — color of the progress bar: the cover's accent, darkened. Hue buckets (12) over pixels with saturation ≥ 0.25,
    weighted by saturation × value; the heaviest bucket's mean color → HSV with saturation ≤ 0.55, value 0.28. Gray `#383838`
    without a cover or for a gray one; eases to the new color at 60 Hz (~1 s) on a cover change.
  - `np_skip` (`1`/`-1`/`0`), `np_skip_x` (`skip.go`) — a ⏩/⏪ that flies across the bottom cell of the left strip in 800 ms (88 % wide)
    (left → right for next, right → left for prev); the TRACK knob posts `/nowplaying/next`, `/nowplaying/prev` on `:47113` with the media key.
    Icons: Bootstrap `fast-forward-fill` / `rewind-fill` with a blurred black outline (`glow()` in `gen_pages.py`).
  - `np_art` — cover of the current track as a JPEG data URI (empty = no cover): the perl child takes
    `kMRMediaRemoteNowPlayingInfoArtworkData`, thumbnails it to ≤160 px JPEG with ImageIO once per new image. Yandex Music gives 150×150 JPEG (verified);
    Chrome/YouTube and Music.app not checked yet.
  - `np_glyph` (0–100, `cover.go`) — opacity of the play/pause glyph over the cover on тайл ряд 2, колонка 1: paused → ▶ stays at 100;
    playing → ⏸ fades in on a state change or touch (`POST /nowplaying/show` on `:47113`, the tile's `down`), holds 1.5 s, fades out.
    60 Hz ticker only while it moves; the glyph also grows 80 → 100 % with it. Glyphs are Bootstrap `play-fill` / `pause-fill` through `glow()`
    (two `feGaussianBlur`-ed black copies under a white path; Skia renders SVG filters). No cover → the old text glyph.
    Cost per frame in Companion's Skia: ~0.4–0.6 ms for cover + glyph (measured with `@napi-rs/canvas` from the app bundle).
- Idle CPU ~0 % (measured on the Go daemon): the 60 fps ticker runs only while the bar moves; no audio poll: CoreAudio listeners
  (`AudioObjectAddPropertyListenerBlock`, `audio_darwin.c`) on the default output device's volume and mute, plus one on the default
  device itself that moves them to the new device (verified 2026-11-04: `osascript` volume/mute changes reach `volume` in < 0.3 s).
- Every 10 s it re-sends all variables, so a Companion restart heals itself.

### Why perl

Since macOS 15.4 MediaRemote returns nothing (`MRMediaRemoteGetNowPlayingInfo` → nil) to third-party processes.
`/usr/bin/perl` is Apple-signed with the right entitlement; loading our dylib into it with `DynaLoader` and calling an XSUB
entry point (`nowplaying_main`) gets the data. Build the dylib for `arm64` and `arm64e` (perl is arm64e).
`zored_stream` runs `/usr/bin/perl ~/.local/share/zored-stream/nowplaying.pl ~/.local/lib/libnowplaying.dylib` as a child
(`tools/zored_stream/nowplaying/`), restarts it 1 s after it exits; the child exits when its parent does.
Same trick as [ungive/mediaremote-adapter](https://github.com/ungive/mediaremote-adapter).

## Things that need the user

Everything else (`make zored_stream`, `make module`, `make deploy`) sessions run themselves. These need the user:

- `make zored_stream-cert` (keychain password)
- `make companion` (adds a login item)
- `make agent-hooks` (writes `~/.claude/settings.json`)
- granting Accessibility to Companion and `zored_stream`, Automation for Amphetamine and iTerm2, Calendars + Reminders (full access) to `zored_stream`
- Bluetooth for `zored_stream` (headphones), Automation for Viscosity and System Events (wi-fi) *(assumed: prompts not recorded)*
- `sudo ipconfig setverbose 1` once (wi-fi SSID), `brew install m1ddc` (KVM, `/opt/homebrew/bin/m1ddc`)

## Sleep

The device has no HID interface, so it can't wake a sleeping Mac (verified: presses do nothing while asleep, the status LED stays on).

- System sleep: Companion logs `Power: Suspending` and closes the surface; the module fork's `close()` blanks the device (verified: black while asleep, 2026-11-11 logs), then draws a white crescent on тайл ряд 1, колонка 4 (`src/moon.ts`, the Bootstrap moon-fill as two circles, no SVG renderer). On `Power: Resuming` Companion reopens the surface, blanks it and redraws the pages. The daemon can't draw it: Companion tears the surface down in the same milliseconds as the sleep notice. Any other close (Companion quit, module reload) leaves the moon too. Verified 2026-11-12 on the device.
- Screens off (display sleep, the Mac awake): `power.go` asks `CGDisplayIsAsleep` for every online display each second; all asleep → `screen_off = 1` and `power_svg` = the same white moon centered. Every other MEDIA control gets a top black box and a black LED while `screen_off == 1` (`gen_pages.py` `dark_when_screen_off`). A press still runs its action and wakes the screen (any daemon POST declares activity). Verified 2026-11-12 on the device.
- Lock screen: `CGSessionCopyCurrentDictionary` `CGSSessionScreenIsLocked` each second → `power_svg` = a white `lock-fill`, centered. Verified 2026-11-12 on the device. Order: hold-to-sleep moon, screens-off moon, lock, then the sun.

## zored_stream — headphones

`headphones.go`, `headphones_darwin.{go,m,h}`, `:47123`: `POST /headphones/toggle` from тайл ряд 1, колонка 1; variable `headphones_svg`
= the whole tile (speaker or headphones glyph, yellow while switching, red 3 s on failure). State is polled every 2 s:
headphones = the default output is any Bluetooth CoreAudio device; a press then goes back to speakers.

- On: `IOBluetoothDevice openConnection` to `80:C3:BA:5F:A4:76` (Sennheiser Accentum), wait up to 10 s for its CoreAudio device,
  then default output + input = Accentum. Off: output = a device named `Logi*`, else built-in; input = a USB device, else built-in;
  then `closeConnection`. The toggle returns at once, the switch runs in the background.
- Auto route: each poll compares devices (`transport/name`) with the previous poll; a new `Logi*` output or USB input while the
  output isn't Bluetooth → the speakers route. Only on arrival, so a manual pick stays. The monitor's DisplayPort output is never picked.
- IOBluetooth needs the Bluetooth permission (`kTCCServiceBluetoothAlways`); without `NSBluetoothAlwaysUsageDescription` it fails
  silently (verified). With it, `openConnection` reaches the headset (verified: `0xe00002d6` timeout while it was off). `Info.plist` is linked into the binary (`-sectcreate __TEXT __info_plist`).
  First press: macOS asks to allow Bluetooth for `zored_stream`; until then the tile stays yellow.
- No `blueutil` / `SwitchAudioSource`: all in-process.

## zored_stream — KVM

`kvm.go`, `:47125`: `POST /kvm/windows` (tap) and `/kvm/pip` (hold) from кнопка 1 (choice-gated); variable `kvm_led` = pink, white for 0.3 s on press.
Tap sends a Wake-on-LAN magic packet to `ec:3a:56:fa:39:ac` (PC at 192.168.4.211) via `192.168.4.255:9`, then
runs `/opt/homebrew/bin/m1ddc set input 15` (MSI MPG 321URX, VCP 60: 15 = DisplayPort = Windows; USB-C 27, HDMI 17/18);
the monitor's KVM then moves USB, the controller included. No way back from the Mac side. `m1ddc get input` returns `0`
on this monitor (verified), so the button shows no state.
Hold toggles PIP with `m1ddc set pbp 33` / `0` (Dell VCP E9; DDC write succeeds on the MSI, effect not confirmed); the daemon
keeps the on/off state itself, so a change from the monitor menu puts it out of step.

## zored_stream — keep awake and sleep

`power.go`, `power_darwin.go`, `:47122`: `POST /power/down`, `/power/up` from тайл ряд 1, колонка 4; variable `power_svg` = the whole tile
(sun hollow / filled, time left in JetBrains Mono, moon while armed).

- Keep-awake = an [Amphetamine](https://apps.apple.com/app/amphetamine/id937984704) session, driven by `osascript`
  (`Amphetamine.sdef`): `start new session with options {duration:N, interval:minutes, displaySleepAllowed:false}`, `end session`,
  `session is active`, `session time remaining`. The session shows in Amphetamine's menu; ending it there turns the tile off.
- State comes from Amphetamine. Every 2 s the daemon checks in-process (`IOPMCopyAssertionsByProcess`, no fork) for an assertion named `Amphetamine…`; `osascript` runs only when that flips, or every 30 s while a session is on (external extensions) (`-3` remaining = no session, `0` = infinite: sun without time,
  `-1`/`-2` = Trigger / app session: same). Time left ticks locally every second between polls.
- Press = 1 h session or end. For 5 s after turning on, `/brightness/up|down` (`:47118`) set the length instead: ±15 min with the knob
  acceleration, 15 min – 12 h, each turn restarts the 5 s; 500 ms after the last turn the session restarts with the new length
  (verified: 1:00 → 1:45, Amphetamine 6300 s, display sleep not allowed).
- First run: macOS asks to let `zored_stream` control Amphetamine (Automation); until allowed, `osascript` hangs and the tile stays off.
- Amphetamine must be running *(assumed: `tell application` launches it otherwise)*.
- Tap off first takes the same 5 s `PreventUserIdleDisplaySleep` bridge as a start, then ends the session, then declares activity: `osascript end session` takes hundreds of ms, and with HID idle past `displaysleep` the screen went dark in that gap (`pmset -g log` 2026-11-12 00:19, 00:34: "Display is turned off" → "on" 2 s later, no system sleep) *(assumed fix: not yet confirmed on the device)*.
- Tap off also calls `IOPMAssertionDeclareUserActivity`: the session's display assertion hid the idle timer, so with no keyboard
  or mouse input for longer than `displaysleep` the screen went dark the moment the session ended and looked like sleep
  (verified in `pmset -g log`: Amphetamine release and "Display is turned off" in the same second, no system sleep).
- Any controller input keeps the Mac awake: every `POST` to any daemon port (except `/agent/event`, the Claude hooks) calls
  a zero-move `kCGEventMouseMoved` at the cursor (resets `HIDIdleTime`, verified 6.6 s → 0.4 s) plus `IOPMAssertionDeclareUserActivity` (a fresh "zored_stream controller" each time). `UserIsActive` alone does not reset the display idle timer: with HID idle past `displaysleep` the screen still went dark the moment Amphetamine released its assertion (tap off, length change; 2026-11-04/05), at most once per 5 s
  (`power.go` `touched`; verified in `pmset -g assertions`). Every control posts (`curl` or the fork's `fetch`) except a bare tap on a
  strip cell (cols 1, 6) without a drag.
- Every session start or length change first takes its own `PreventUserIdleDisplaySleep` ("zored_stream keep-awake restart",
  released after 5 s): "start new session" releases the old display assertion before creating the new one, and with HID idle past
  `displaysleep` the screen went dark in that gap (`pmset -g log` 2026-11-04 22:20, 22:25, 2026-11-05 00:30: Amphetamine Released →
  "Display is turned off" in the same second, back on 2–4 s later, no system sleep). A fresh `UserIsActive` declared right before did
  not stop it (00:30:56).
- Closing the lid sleeps the Mac anyway unless Amphetamine's closed-display mode is on; not set here.
- Sleep = end the session, then `IOPMSleepSystem` (cgo), no root, no `pmset sleepnow` *(assumed: not triggered during development)*.
- Timing lives in the daemon (press 1 s → `arm`, 3 s → cancel), not in Companion's 400 ms hold, so no second haptic.
- `/location/…/press` over the API sends down and up at once and they race; test with `/down` + `/up`.

## zored_stream — brightness

`brightness.go`, `display_darwin.go`; HTTP on `127.0.0.1:47118` (`-brightness`). Крутилка справа, ряд 1 (was слева, ряд 3):
turn = Mac display brightness, turn while held = the controller's own brightness.

- `POST /brightness/up`, `/brightness/down` (same acceleration as volume), `/brightness/hold` (press), `/brightness/release`; every reply is the controller
  brightness as plain text.
- Mac: `DisplayServicesGetBrightness` / `SetBrightness` (private `DisplayServices.framework`, main display, 0–1). Read back on every
  turn, so keyboard changes are picked up. Verified: turns move the built-in display. External monitors (DDC) not done.
- Controller: Companion's internal `set_brightness` (surface `self`) inside a sequential `action_group`, after the `exec` that
  writes the daemon's reply into `ctl_brightness`. Companion stores it in the surface config (`db.sqlite`, `surfaces.config.brightness`),
  so it survives restarts; the daemon reads it once at start (`/usr/bin/sqlite3 -readonly`, `-companion-db`), and a change made in
  the Companion UI is not seen until the daemon restarts. Floor 10 %, so the device never goes dark.
  A press via the HTTP API has no surface, so `self` resolves to nothing there: test on the device.
- The bar shows the active target for 1 s: white = Mac, gray = controller; then back to volume. The knob press itself is the haptic;
  no haptic at 0/100 (the module fork has press/hold/slide only).

## zored_stream — seek

`seek.go`; HTTP on `127.0.0.1:47119` (`-seek`): `POST /seek/forward`, `/seek/back`. Крутилка слева, ряд 3.

- A turn moves a target position: 2 / 4 / 8 / 15 s per detent by tick interval (the volume acceleration), clamped to
  0 … duration − 1 s. The first turn after a pause of > 1 s starts from the extrapolated current position.
- The shared bar shows the target bright white at 60 fps (`bar_mode` `seek`), fades out 1 s after the last turn; the dim
  progress then continues from the new position.
- Live seek: the daemon writes `seek <seconds>` to the perl child's stdin (at most every 150 ms, the last target always goes out);
  the child calls `MRMediaRemoteSetElapsedTime`, which goes to the current Now Playing app only. Stdin EOF → the child exits.
- Players: Yandex Music seeks (verified, 25 → 30 → 90 s). Chrome/YouTube, Music.app not checked yet.
- No haptic at the track start/end: the module fork only vibrates on press/hold/slide, the daemon can't trigger it.

## zored_stream — scroll

`scroll.go`, `scroll_darwin.go`; HTTP on `127.0.0.1:47121` (`-scroll`): `POST /scroll/forward`, `/scroll/back`, `/scroll/down` (press),
`/scroll/up` (release). Крутилка справа, ряд 3: turn = vertical wheel, turn while held = horizontal. No strip, no haptic.
Verified on the device by the user (2026-11-02).

Tab mode: a click (down + up with no turn in between) toggles wheel ↔ tab, stored in `~/.local/share/zored-stream/scroll-mode`
(`-scroll-mode-file`, `wheel` / `tab`, missing = wheel, anything else = the daemon exits). In tab mode the first detent posts
Ctrl down, each detent a Tab tap with the Control flag (+ Shift counter-clockwise), and Ctrl up 500 ms after the last detent.
Ctrl is also released on knob press, SIGINT/SIGTERM, and posted up once at start (covers a crash with Ctrl down).

- `CGEventCreateKeyboardEvent` (Control = 59, Tab = 48) with `CGEventSetFlags`, posted to `kCGHIDEventTap`, so it goes
  to the frontmost app. The toggle has no own haptic: the press tap is the cue.
- Verified: the click toggles and persists, press + turn keeps the mode, all endpoints answer `204`.
- Delivery to apps verified on the device by the user (2026-11-03). `CGEventSourceFlagsState` doesn't show synthetic Ctrl,
  and a listen-only event tap gets nothing without Input Monitoring, so there's no local check. Ctrl+Tab does nothing
  at the macOS level; app switching is Cmd+Tab.

App switching: in tab mode, press + turn holds Cmd and taps Tab (+ Shift counter-clockwise), the system Cmd+Tab switcher.
Cmd goes up on knob release, which picks the app; a 5 s timer, SIGINT/SIGTERM and the start release it too. For 3 s after
that release (each detent extends it) a plain turn posts Cmd+` (+ Shift) instead of Ctrl+Tab, cycling windows of the picked
app; Cmd is held between those detents and released 500 ms after the last, like Ctrl. Contexts (installed) ignores synthetic
`CGEventPost` keys, tried five event variants; its search hotkey and other entry points don't help either (no URL scheme,
no AppleScript). The Karabiner VirtualHID route needs a root client and was not pursued.

- `CGEventCreateScrollWheelEvent` in line units (like a real wheel detent), 1 / 2 / 4 / 8 lines per detent by tick interval
  (the volume acceleration), posted to `kCGHIDEventTap`, so it lands in the window under the cursor, as a mouse wheel does.
- Direction is fixed: clockwise = further down / right through the document, regardless of natural scrolling
  (synthetic events skip the HID inversion; assumed). Natural scrolling here is off (`com.apple.swipescrolldirection` = 0).
- Permission: `CGPreflightPostEventAccess` is true for the signed binary, the daemon's Accessibility grant covers it (verified).
  Without it the turn returns `409` with `no permission to post events`.

## Wi-fi (`wifi.go`, :47124)

- Power: CoreWLAN.
- Every SSID source is redacted without Location: `ipconfig`, `system_profiler`, `wdutil`, CoreWLAN scan (verified). A bare binary with `NSLocation*UsageDescription` in the embedded `Info.plist` gets an `#AuthPrompt` in the `locationd` log but no window (verified). A separate `LSUIElement` app with its own grant did scan with names, but its list mixed unknown networks and missed Instant Hotspots; dropped.
- Current SSID: `sudo ipconfig setverbose 1` once, then `ipconfig getsummary en0` prints it (verified). Survives a reboot *(unknown)*.
- Hotspot: the router of an iPhone Personal Hotspot is in 172.20.10.0/28 *(assumed, Apple default)*; the tile draws a phone then.
- List and join go through Control Center via `osascript` (`wifiMenu` in `wifi_darwin.go`): menu bar item whose description starts with "Control Center" (it becomes "Control Center, Microphone is in use", so no `is`; and `click` on a loop reference does nothing, `AXPress` by index works) → tile `controlcenter-wifi` → action "show details" → checkboxes `wifi-network-<ssid>`: hotspots, then known networks in range (verified, ~2.8 s). Join clicks the checkbox; macOS fills the password itself (verified on `zored_5G`). API joins fail: `networksetup -setairportnetwork`, `associate(password:nil)` → `tmpErr`; the System keychain asks for the admin password.
- Viscosity: AppleScript `connections` (`name`, `state`: `Connected`, `Disconnected`, others = in progress), `connect <name>`, `disconnectall`; the state script checks `application "Viscosity" is running` first so polling never launches it. The 3 s poll forks it only when a utun with IPv4 appears or disappears (`vpnTunnel`, `net.Interfaces`; system utuns carry only IPv6, verified 2026-11-03: `utun10 172.20.93.9`), while a state is in progress, or every 30 s. Switch order: CoreWLAN `disassociate` → `disconnectall` → menu join → wait until `ipconfig` shows the new SSID with a router (≤ 15 s) → 1 s → `connect` the `-wifi-vpn` connection (default `Barbos_SE`), wait ≤ 20 s for `Connected`, one retry. Restoring "whatever was up" failed live: the state read at switch time could already be down, and 1 s after the join the link was not ready.
- Control Center is flaky when reopened right after closing: the menu bar item toggles, so the script first waits up to 2.5 s for its window to close, then presses; the list is read until two polls match; one retry in Go; calls are serialized by a mutex (two joins at once used to toggle it shut). With 2 s between calls 3/3 runs passed, back to back 1/4 (verified).
- VPN restore runs even when the join fails (it used to return early and leave Viscosity down).
- AppleScript: iterate a list stored in a variable (`set nodes to entire contents …`), not `repeat with e in (entire contents …)` — the latter re-queries and finds nothing (verified).

## Keyboard layout (`input.go`, :47127)

- Text Input Sources in-process (`input_darwin.m`): `POST /input/next` selects the next enabled select-capable keyboard source (`TISCreateInputSourceList`, palette/dictation/press-and-hold are not keyboard sources); variable `input_svg` = the first language of the current source (`RussianWin` → `ru`).
- No polling: `kTISNotifySelectedKeyboardInputSourceChanged` on the distributed center. Both the notification and the `TISCopyCurrentKeyboardInputSource` cache need the **main thread** run loop: on a background thread the callback never fires and the current source stays stale (verified with a probe). So `input_darwin.go` locks `main` to the main thread in `init` and `main` ends in `CFRunLoopRun`; every HTTP server runs in a goroutine.

## Calendar (`calendar.go`, :47126)

- EventKit in-process (`calendar_darwin.m`): events ±7 days (cancelled dropped) and incomplete reminders with a due date up to +7 days, polled every minute; past events drop out, overdue reminders stay.
- Access: `requestFullAccessTo{Events,Reminders}` (macOS 14+), usage strings in the embedded `Info.plist`; the first poll blocks until the prompt is answered.
- Notes (`EKCalendarItem.notes`) go to the list strip, gray.
- Soon glow: the day tile gets a dark green background (`soonBgRGB`, `cal_canvas_fill`) while the first non-reminder event is ≤ 5 min ahead or running (`calGlow`); a tap, knob turn or act stores its id+start in `dismissed`, so it never comes back for that event. The glow is part of the card's frame key: re-render only on the edge, at most 1 min late (poll).
- Done toggles `EKReminder.completed` + `saveReminder:commit:`; reminders completed since midnight are listed too.
- Text is drawn by CoreText (`cal_canvas_*`, JetBrains Mono, system emoji fallback, char wrap) into a 4× PNG: Companion's SVG text has no emoji. The frame path is taller than the box and clipped, otherwise a line taller than the box (emoji ascent) is dropped; lines are drawn one by one at a fixed baseline (font ascent, step = 1.25 × size) so an emoji does not push its line down. Open: `open ical://ekevent/<eventIdentifier>?method=show&options=more`.
- Knob capture: `/scroll/*` ask `calendar` first (`knob`, `turn`); entering the list releases Ctrl (S1).
- The tile tap goes to `/tile/24/press`: the `slot` in `layers.go` routes it to hang-up during a call, else to the calendar (F23).
- кнопка 5 posts `/button/5/press`: ignored during a call, else the same `press`. LED `cal_led`: green while browsing, else white; the page shows 0 while `tile_24` is `call`.

## Timers (`timers.go`, :47129)

- `/timer/down` + `/timer/up`: tap = edit / start / silence, hold ≥ 400 ms = cancel the nearest; `/scroll/*` ask `timers` first (`knob`, `turn`), then `calendar`; entering the edit releases Ctrl (S1).
- Ticks every 250 ms: idle edit commits, due timers drop out and ring (`timer_ring`, `afplay Glass.aiff` every 2 s, 1 min); `timer_svg` is the line under the clock.
- No Clock.app timers: Shortcuts can start one (`Start Timer`), but nothing reads the running ones back, so the tile could not show them; timers are the daemon's own, in `~/.config/zored-stream/timers.json`. No haptic: the daemon cannot drive the motor.
