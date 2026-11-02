# Plan

Facts live in `docs/`; this file tracks status.

## How work runs

A master session keeps this file; each feature goes to its own child session.

- Parallel-safe: anything that doesn't touch `tools/gen_pages.py` or run `make deploy` (CI, upstream PR, docs, daemons until wiring).
- One at a time: sessions that edit `gen_pages.py` or deploy. `make deploy` overwrites pages 1–2 in the one running Companion,
  so two deploying sessions wipe each other's buttons.
- Take free controls from the map below and update it in the same commit.

MEDIA page, `row/col` ([grid](docs/companion.md)):

| Used | Free |
|---|---|
| left knobs `0/0` volume, `1/0` track (hold press = like), `2/0` brightness (claimed); left strip `0–2/1` volume | — |
| tiles `1/2` play (hold = like), `2/2` mic, `2/3` camera, `2/4` hang up, `2/5` clock | tiles `0/2–0/5`, `1/3–1/5` |
| buttons `3/1` mic, `3/2` camera, `3/3` hang up | buttons `3/0`, `3/4–3/7` |
| right strip `0–2/6` AI limits + model, right knob `1/7` AI model | right knobs `0/7`, `2/7` |

Idea: mic, camera and hang-up tiles do something else outside a call.

## Done (2026-11-01)

- Device, firmware 0.2.14, protocol events verified; custom firmware researched and parked ([device.md](docs/device.md))
- Companion 5.1.1 + Loupedeck surface module; every control and LED verified with the TEST page
- Pages as code: `gen_pages.py` → `make deploy` over tRPC, no UI clicks
- MEDIA page: volume knob with acceleration, track knob, play/pause tile with OS state, clock, track on the strip rotated 90°,
  animated volume bar, strip swipe volume, haptics ([companion.md](docs/companion.md), [macos.md](docs/macos.md))
- `mediakey`, `streamd` (LaunchAgent) installed on the Mac

## Done (2026-11-02)

- VK WorkSpace calls: buttons 2/3 toggle mic/camera, push-to-talk, button 4 hangs up; LEDs + tiles from the client's real state
  via its local gRPC, `pending` while the camera starts ([vkcall.md](docs/vkcall.md))
- Ideas backlog collected (below)
- Track strip: `zored_stream` `np_svg` — gray artist | bold title (two lines when long), like mark inside; clock `2/5` stretched SVG + gray `DD.MM` date
- AI: Claude limits in `zored_stream` (`usage`, every 15 min, no CLI), right strip = limits + model/effort, right knob row 2 switches the model;
  Hammerspoon overlay reads the daemon ([ai.md](docs/ai.md), [ai-switch.md](docs/ai-switch.md))
- Brightness: left knob row 3 turns Mac brightness, press+turn the controller's; level on the left-strip bar
- Animations at 60 fps (the ceiling through Companion variables, [companion.md](docs/companion.md))
- Yandex Music like: hold play tile / knob 2, second haptic at 400 ms, ♥/💔 on the strip from the app's player bar
  via Accessibility (`ymusic`); MediaRemote, local API, deeplinks, hotkeys are dead ends ([ymusic.md](docs/ymusic.md))

## Next

### macOS polish

- [x] Disable Loupedeck autostart: `launchctl disable gui/$(id -u)/com.logi.pluginservice.launch` (undo: `enable`)
- [x] Companion autostart at login, headless: `make companion`
- [ ] Restore test from scratch: fresh Companion → README setup → works
- [x] Fork of companion-surface-loupedeck, branch `zored_stream`, cloned next to this repo; `make module` replaced `patch-module`;
  node-loupedeck not needed; no upstream PR, changes stay in the fork
- [x] Gentle haptic ticks while sliding along a strip: `SHORT_LOWER` every 10px

### More buttons and actions

New features go on the MEDIA page as buttons; separate pages only once it runs out of room.


- [ ] Keystrokes on macOS (candidates: `osascript` via `exec`, or post key events like `mediakey`)
- [ ] `vkcall`: reconnect after a VK WorkSpace restart — not verified yet
- [ ] IDE / terminal pages; page switching by frontmost app (`zored_stream` can watch `NSWorkspace` and push a variable)
- [ ] Integrations worth a look: OBS, Zoom, GitHub/CI status via HTTP

### Ideas (2026-11-02, mechanisms assumed, not checked)

Desk and display:
- [ ] Keep-awake toggle with time left on the tile: Amphetamine via AppleScript, or `caffeinate -t` owned by `zored_stream`
- [ ] Monitor brightness, input switch (KVM), PiP: DDC/CI via `m1ddc` or BetterDisplay CLI
- [ ] Sleep buttons: display off (`pmset displaysleepnow`), lock, system sleep
- [ ] Headphones toggle: connect/disconnect Sennheiser Accentum over Bluetooth (`blueutil` or IOBluetooth in `zored_stream`) and move output + mic to them;
  back: Logi + USB camera if present, else MacBook (`SwitchAudioSource` or CoreAudio in `zored_stream`); LED shows the active route

Windows and apps:
- [ ] Window switcher on a knob (Alt+Tab / Ctrl+Tab): needs keystrokes (above); hold modifier while turning, release on knob stop
- [ ] Timers and alarm on one knob: turn sets a duration or a wall-clock time (press+turn toggles), press starts;
  countdown / alarm time on the strip, gray dot on the clock tile while armed, haptic + sound at the end; owned by `zored_stream`
- [ ] Keyboard layout on a tile (EN/RU), press switches: Text Input Sources (`TISCopyCurrentKeyboardInputSource`, `TISSelectInputSource`,
  change notification) in `zored_stream`
- [ ] Emoji picker: knob scrolls a short favorites list on the strip, press types the emoji into the focused field
  (`CGEventKeyboardSetUnicodeString` in `zored_stream`, no clipboard)
- [ ] Media seek on a knob (free right knob, row 1 or 3): MediaRemote `SetElapsedTime` / seek commands via the perl Now Playing host; position on the strip
- [ ] Frontmost app icon on a tile, press cycles its windows

AI agents:
- [ ] Dictation into the prompt: hold a button and speak (Superwhisper or macOS dictation), text lands in the agent input
- [ ] Agent waiting for me: Claude Code `Notification`/`Stop` hooks → Companion variable → blinking LED with the count; press focuses the session

Communication:
- [ ] Calendar: next meeting countdown on the strip, LED turns orange 5 min before, press joins the call
- [ ] Unread Telegram / VK WorkSpace: Dock badge via AX (`AXStatusLabel`), works for any app; press opens the app

Helpers:
- [x] One daemon `zored_stream` (Go): calls, volume, Now Playing, Yandex Music like; `vkcall`/`streamd`/`ymusic` removed ([daemon.md](docs/daemon.md))

Network:
- [ ] Viscosity VPN on/off with status color: Viscosity AppleScript (`connect`, `disconnect`, connection state)
- [ ] Current Wi-Fi network on a tile: SSID needs Location permission on recent macOS, `zored_stream` via CoreWLAN

### Windows

- [ ] Plan in [windows.md](docs/windows.md): `streamd.exe` (C#) with the same HTTP endpoints and variables, OS-aware `gen_pages.py`
- [ ] Check whether the device wakes a sleeping Windows PC

### Parked

- Like for other players: MediaRemote `LikeTrack` (21) works only with players implementing it (Music.app); Chrome's Media Session has no like
- Custom firmware ([device.md](docs/device.md))
- Wake from sleep: the Mac can't be woken by the device (no HID); not needed

### CI

- [x] GitHub Actions: `make pages` is deterministic → fail if `companion/` differs; build `mediakey`/`streamd` on `macos-latest`
