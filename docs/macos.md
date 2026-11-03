# macOS helpers

`mediakey` (Companion `exec`) and the `zored_stream` daemon (HTTP) back the Companion pages.

## mediakey — play / next / prev

`tools/mediakey/main.swift`, `make mediakey` → `~/.local/bin/mediakey <play|next|prev|volup|voldown|mute>`.

- Posts system-defined media key events (`NSEvent` subtype 8 → `CGEvent.post(.cghidEventTap)`): reaches whatever owns
  Now Playing, browser tabs included, like a real media key.
- Needs **Companion** in System Settings → Privacy & Security → Accessibility (the child process inherits it).
  Without it the helper exits 1 and Companion logs `Shell command failed … no Accessibility permission`.
- Gotcha: the process must sleep ~100 ms after posting. Exiting right away drops the events silently, even with every permission check passing.
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
    polled every 0.5 s by the perl child (below), sent to the daemon as JSON lines on change
  - `np_progress` (0–100) — playback position, drawn bottom-up in `np_tint` behind the track text on the left strip.
    The perl child sends `duration`, `elapsed`, `rate`, `at` (unix time of `elapsed`) from MediaRemote; the daemon extrapolates
    `elapsed + (now − at) × rate` every 3 s, so no extra polling; paused or no duration → the value doesn't change, nothing is sent.
  - `np_tint` (`tint.go`) — color of the progress bar: the cover's accent, darkened. Hue buckets (12) over pixels with saturation ≥ 0.25,
    weighted by saturation × value; the heaviest bucket's mean color → HSV with saturation ≤ 0.55, value 0.28. Gray `#383838`
    without a cover or for a gray one; eases to the new color at 60 Hz (~1 s) on a cover change.
  - `np_skip` (`1`/`-1`/`0`), `np_skip_x` (`skip.go`) — a ⏩/⏪ that flies across the bottom cell of the left strip in 800 ms (88 % wide)
    (left → right for next, right → left for prev); the TRACK knob posts `/nowplaying/next`, `/nowplaying/prev` on `:47113` with the media key.
    Icons `tools/icons/skip-{forward,back}.svg`.
  - `np_art` — cover of the current track as a JPEG data URI (empty = no cover): the perl child takes
    `kMRMediaRemoteNowPlayingInfoArtworkData`, thumbnails it to ≤160 px JPEG with ImageIO once per new image. Yandex Music gives 150×150 JPEG (verified);
    Chrome/YouTube and Music.app not checked yet.
  - `np_glyph` (0–100, `cover.go`) — opacity of the play/pause glyph over the cover on тайл ряд 2, колонка 1: paused → ▶ stays at 100;
    playing → ⏸ fades in on a state change or touch (`POST /nowplaying/show` on `:47113`, the tile's `down`), holds 1.5 s, fades out.
    60 Hz ticker only while it moves; the glyph also grows 80 → 100 % with it. Glyphs are `tools/icons/{play,pause}-glow.svg`
    (two `feGaussianBlur`-ed black copies under a white path; Skia renders SVG filters). No cover → the old text glyph.
    Cost per frame in Companion's Skia: ~0.4–0.6 ms for cover + glyph (measured with `@napi-rs/canvas` from the app bundle).
- Idle CPU ~0 % (measured on the Go daemon): the 60 fps ticker runs only while the bar moves; audio polled every 0.25 s.
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
- granting Accessibility to Companion and `zored_stream`, Automation for Amphetamine and iTerm2

## Sleep

The device has no HID interface, so it can't wake a sleeping Mac (verified: presses do nothing while asleep, the status LED stays on).

## zored_stream — headphones

`headphones.go`, `headphones_darwin.{go,m,h}`, `:47123`: `POST /headphones/toggle` from тайл ряд 1, колонка 3; variable `headphones_svg`
= the whole tile (speaker or headphones glyph, gray while switching, red 3 s on failure). State is polled every 2 s:
headphones = the default output is any Bluetooth CoreAudio device; a press then goes back to speakers.

- On: `IOBluetoothDevice openConnection` to `80:C3:BA:5F:A4:76` (Sennheiser Accentum), wait up to 10 s for its CoreAudio device,
  then default output + input = Accentum. Off: output = a device named `Logi*`, else built-in; input = a USB device, else built-in;
  then `closeConnection`. The toggle returns at once, the switch runs in the background.
- IOBluetooth needs the Bluetooth permission (`kTCCServiceBluetoothAlways`); without `NSBluetoothAlwaysUsageDescription` it fails
  silently (verified). With it, `openConnection` reaches the headset (verified: `0xe00002d6` timeout while it was off). `Info.plist` is linked into the binary (`-sectcreate __TEXT __info_plist`).
  First press: macOS asks to allow Bluetooth for `zored_stream`; until then the tile stays gray.
- No `blueutil` / `SwitchAudioSource`: all in-process.

## zored_stream — keep awake and sleep

`power.go`, `power_darwin.go`, `:47122`: `POST /power/down`, `/power/up` from тайл ряд 1, колонка 4; variable `power_svg` = the whole tile
(sun hollow / filled, time left in JetBrains Mono, moon while armed).

- Keep-awake = an [Amphetamine](https://apps.apple.com/app/amphetamine/id937984704) session, driven by `osascript`
  (`Amphetamine.sdef`): `start new session with options {duration:N, interval:minutes, displaySleepAllowed:false}`, `end session`,
  `session is active`, `session time remaining`. The session shows in Amphetamine's menu; ending it there turns the tile off.
- State comes from Amphetamine, polled every 2 s (`-3` remaining = no session, `0` = infinite: sun without time,
  `-1`/`-2` = Trigger / app session: same). Time left ticks locally every second between polls.
- Press = 1 h session or end. For 5 s after turning on, `/brightness/up|down` (`:47118`) set the length instead: ±15 min with the knob
  acceleration, 15 min – 12 h, each turn restarts the 5 s; 500 ms after the last turn the session restarts with the new length
  (verified: 1:00 → 1:45, Amphetamine 6300 s, display sleep not allowed).
- First run: macOS asks to let `zored_stream` control Amphetamine (Automation); until allowed, `osascript` hangs and the tile stays off.
- Amphetamine must be running *(assumed: `tell application` launches it otherwise)*.
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

- `CGEventCreateScrollWheelEvent` in line units (like a real wheel detent), 1 / 2 / 4 / 8 lines per detent by tick interval
  (the volume acceleration), posted to `kCGHIDEventTap`, so it lands in the window under the cursor, as a mouse wheel does.
- Direction is fixed: clockwise = further down / right through the document, regardless of natural scrolling
  (synthetic events skip the HID inversion; assumed). Natural scrolling here is off (`com.apple.swipescrolldirection` = 0).
- Permission: `CGPreflightPostEventAccess` is true for the signed binary, the daemon's Accessibility grant covers it (verified).
  Without it the turn returns `409` with `no permission to post events`.
