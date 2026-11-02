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
| left knobs `0/0` volume, `1/0` track (hold press = like); left strip `0–2/1` volume | left knob `2/0` |
| tiles `0/2` play (hold = like), `2/2` mic, `2/3` camera, `2/4` hang up, `2/5` clock | tiles `0/3–0/5`, `1/2–1/5` |
| buttons `3/1` mic, `3/2` camera, `3/3` hang up | buttons `3/0`, `3/4–3/7` |
| | right knobs `0–2/7`, right strip `0–2/6` |

## Done (2026-11-01)

- Device, firmware 0.2.14, protocol events verified; custom firmware researched and parked ([device.md](docs/device.md))
- Companion 5.1.1 + Loupedeck surface module; every control and LED verified with the TEST page
- Pages as code: `gen_pages.py` → `make deploy` over tRPC, no UI clicks
- MEDIA page: volume knob with acceleration, track knob, play/pause tile with OS state, clock, track on the strip rotated 90°,
  animated volume bar, strip swipe volume, haptics ([companion.md](docs/companion.md), [macos.md](docs/macos.md))
- `mediakey`, `streamd` (LaunchAgent), `patch-module` installed on the Mac

## Done (2026-11-02)

- VK WorkSpace calls: buttons 2/3 toggle mic/camera, push-to-talk, button 4 hangs up; LEDs + tiles from the client's real state
  via its local gRPC, `pending` while the camera starts ([vkcall.md](docs/vkcall.md))
- Ideas backlog collected (below)
- Yandex Music like: hold play tile / knob 2, second haptic at 400 ms, ♥/💔 on the strip from the app's player bar
  via Accessibility (`ymusic`); MediaRemote, local API, deeplinks, hotkeys are dead ends ([ymusic.md](docs/ymusic.md))

## Next

### macOS polish

- [ ] Disable Loupedeck autostart: `launchctl disable gui/$(id -u)/com.logi.pluginservice.launch` (undo: `enable`)
- [ ] Companion autostart at login, headless
- [ ] Restore test from scratch: fresh Companion → README setup → works
- [ ] Upstream PR to companion-surface-loupedeck: haptics + strip touch position in buttons mode (drops `patch-module`)

### More pages and actions

- [ ] Keystrokes on macOS (candidates: `osascript` via `exec`, or post key events like `mediakey`)
- [ ] `vkcall`: reconnect after a VK WorkSpace restart — not verified yet
- [ ] IDE / terminal pages; page switching by frontmost app (`streamd` can watch `NSWorkspace` and push a variable)
- [ ] Right knobs and right strip: free now
- [ ] Integrations worth a look: OBS, Zoom, GitHub/CI status via HTTP

### Ideas (2026-11-02, mechanisms assumed, not checked)

Desk and display:
- [ ] Keep-awake toggle with time left on the tile: Amphetamine via AppleScript, or `caffeinate -t` owned by `streamd`; ties into wake Plan A
- [ ] Controller brightness on a knob: Companion internal surface brightness action
- [ ] Monitor brightness, input switch (KVM), PiP: DDC/CI via `m1ddc` or BetterDisplay CLI
- [ ] Sleep buttons: display off (`pmset displaysleepnow`), lock, system sleep

Windows and apps:
- [ ] Window switcher on a knob (Alt+Tab / Ctrl+Tab): needs keystrokes (above); hold modifier while turning, release on knob stop
- [ ] Timers on a knob: turn sets the duration, press starts, countdown on the strip, haptic at the end
- [ ] Frontmost app icon on a tile, press cycles its windows

AI agents:
- [ ] AI limits on a tile: Claude Code statusline JSON `rate_limits`, Codex `token_count` events in `~/.codex/sessions`
- [ ] Model + effort switch for Claude Code / Codex: settings files or app keystrokes, research first
- [ ] Dictation into the prompt: hold a button and speak (Superwhisper or macOS dictation), text lands in the agent input
- [ ] Agent waiting for me: Claude Code `Notification`/`Stop` hooks → Companion variable → blinking LED with the count; press focuses the session

Communication:
- [ ] Calendar: next meeting countdown on the strip, LED turns orange 5 min before, press joins the call
- [ ] Unread Telegram / VK WorkSpace: Dock badge via AX (`AXStatusLabel`), works for any app; press opens the app

Network:
- [ ] Viscosity VPN on/off with status color: Viscosity AppleScript (`connect`, `disconnect`, connection state)
- [ ] Current Wi-Fi network on a tile: SSID needs Location permission on recent macOS, `streamd` via CoreWLAN

### Windows

- [ ] Plan in [windows.md](docs/windows.md): `streamd.exe` (C#) with the same HTTP endpoints and variables, OS-aware `gen_pages.py`
- [ ] Check whether the device wakes a sleeping Windows PC

### Wake from sleep (Mac: device can't, no HID)

- [ ] Plan A — keep the system awake, only the display sleeps; any press wakes the display
- [ ] Plan B — tiny HID board (RP2040 / Pro Micro) with a wake key
- [ ] Plan C — Wake-on-LAN from another device

### Parked

- Like for other players: MediaRemote `LikeTrack` (21) works only with players implementing it (Music.app); Chrome's Media Session has no like
- Custom firmware ([device.md](docs/device.md))

### CI

- [ ] GitHub Actions: `make pages` is deterministic → fail if `companion/` differs; build `mediakey`/`streamd` on `macos-latest`
