# Plan

Facts live in `docs/`, rules in [docs/invariants.md](docs/invariants.md); this file tracks status.

## How work runs

A master session keeps this file; each feature goes to its own fresh child session.

- Parallel-safe: anything that doesn't touch `tools/gen_pages.py` or run `make deploy` (CI, docs, daemon modules until wiring).
- One at a time: sessions that edit `gen_pages.py` or deploy. `make deploy` overwrites pages 1–2 in the one running Companion,
  so two deploying sessions wipe each other's buttons.
- Take free controls from the map below and update it in the same commit.

MEDIA page, `row/col` ([grid](docs/companion.md), names in [controls.md](docs/controls.md)):

| Used | Free |
|---|---|
| left knobs `0/0` volume, `1/0` track (hold = like), `2/0` seek; left strip `0–2/1` track + bar | — |
| tiles `0/5` keep-awake, `1/2` play (hold = like), `1/5` agents waiting, `2/2` mic, `2/3` camera, `2/4` hang up, `2/5` clock | tiles `0/2–0/4`, `1/3–1/4` |
| buttons `3/1` mic, `3/2` camera, `3/3` hang up, `3/5` agents, `3/6` dictation; all of `3/0–3/7` in choice mode | buttons `3/0`, `3/4`, `3/7` (outside choice mode) |
| right strip `0–2/6` AI limits + model, right knobs `0/7` brightness, `1/7` AI model, `2/7` scroll / Ctrl+Tab | — |

## Done

- 2026-11-01: device, firmware 0.2.14, protocol events; custom firmware parked ([device.md](docs/device.md)); Companion 5.1.1 +
  Loupedeck module, TEST page; pages as code (`gen_pages.py` → `make deploy` over tRPC); volume, track, play/pause, clock, track strip,
  volume bar, strip swipe, haptics
- 2026-11-02: VK WorkSpace calls ([vkcall.md](docs/vkcall.md)); one Go daemon `zored_stream` replacing `streamd`, `vkcall`, `ymusic`
  ([daemon.md](docs/daemon.md)); Yandex Music like ([ymusic.md](docs/ymusic.md)); Claude limits + model/effort switch
  ([ai.md](docs/ai.md), [ai-switch.md](docs/ai-switch.md)); brightness; cover on the play tile; JetBrains Mono; seek + progress bar;
  module fork with haptics, no upstream PR ([companion.md](docs/companion.md)); Loupedeck autostart disabled, `make companion`
- 2026-11-03: agents waiting + Desktop unread, dictation with music pause/resume, choice mode on кнопки 1–8 ([agents.md](docs/agents.md));
  scroll + Ctrl+Tab mode; keep-awake via Amphetamine + hold-to-sleep ([macos.md](docs/macos.md)); limits token refreshed by the
  daemon, stale values gray; animations at 60 fps; [controls.md](docs/controls.md); Windows plan + `gen_pages.py` profile `mac|windows`
  ([windows.md](docs/windows.md)); CI: concurrency + timeout, pages ↔ daemon contract check, Go tests; [invariants.md](docs/invariants.md)

## Next

### Verify

- [ ] Restore test from scratch: fresh Companion → README setup → works
- [ ] Calls: reconnect after a VK WorkSpace restart
- [ ] Keep-awake hold → sleep (never triggered; ask first)
- [ ] Choice mode: CLI permission prompt in iTerm2 (кнопка 1 = Esc not run)
- [ ] Seek and cover with Chrome/YouTube and Music.app; dictation resume with a browser player
- [ ] Right strip slide picks the model on the device

### More buttons and actions

New features go on the MEDIA page as buttons; separate pages only once it runs out of room.

- [ ] Mic, camera and hang-up tiles do something else outside a call
- [ ] IDE / terminal pages; page switching by frontmost app (`zored_stream` already reads it via `lsappinfo` and can push a variable)
- [ ] Integrations worth a look: OBS, Zoom, GitHub/CI status via HTTP
- [ ] `mediakey` into the daemon as `POST /key/<name>` (drops the `exec` and Companion's Accessibility; Windows needs the route anyway)

### Ideas (mechanisms assumed, not checked)

Desk and display:
- [ ] Monitor brightness, input switch (KVM), PiP: DDC/CI via `m1ddc` or BetterDisplay CLI
- [ ] Display off (`pmset displaysleepnow`), lock
- [ ] Headphones toggle: connect/disconnect Sennheiser Accentum over Bluetooth (IOBluetooth in `zored_stream`) and move output + mic to them;
  back: Logi + USB camera if present, else MacBook (CoreAudio in `zored_stream`); LED shows the active route

Windows and apps:
- [ ] App switcher (Cmd+Tab) on a knob, same release rules as Ctrl+Tab ([invariants.md](docs/invariants.md) S1)
- [ ] Timers and alarm on one knob: turn sets a duration or a wall-clock time (press+turn toggles), press starts;
  countdown / alarm time on the strip, gray dot on the clock tile while armed, haptic + sound at the end; owned by `zored_stream`
- [ ] Keyboard layout on a tile (EN/RU), press switches: Text Input Sources (`TISCopyCurrentKeyboardInputSource`, `TISSelectInputSource`,
  change notification) in `zored_stream`
- [ ] Emoji picker: knob scrolls a short favorites list on the strip, press types the emoji into the focused field
  (`CGEventKeyboardSetUnicodeString` in `zored_stream`, no clipboard)
- [ ] Frontmost app icon on a tile, press cycles its windows

AI agents:
- [ ] Codex sessions on the agents tile ([agents.md](docs/agents.md#next-codex))
- [ ] Haptic on a new waiting session (needs a fork endpoint)

Communication:
- [ ] Calendar: next meeting countdown on the strip, LED turns orange 5 min before, press joins the call
- [ ] Unread Telegram / VK WorkSpace: Dock badge via AX (`AXStatusLabel`), works for any app; press opens the app

Network:
- [ ] Viscosity VPN on/off with status color: Viscosity AppleScript (`connect`, `disconnect`, connection state)
- [ ] Current Wi-Fi network on a tile: SSID needs Location permission on recent macOS, `zored_stream` via CoreWLAN

### Windows

Windows PC has only Codex (no Claude Code/Desktop), its own media players, no VK WorkSpace. The Mac setup stays.

- [x] Plan in [windows.md](docs/windows.md); `gen_pages.py` profile `windows` → `companion/windows/`
- [ ] Next code step: daemon split `modules_darwin.go` / `modules_windows.go`, `*_windows.go` for volume + keys + scroll, CI `windows-latest`
- [ ] Decide the AI knob on Windows: the profile keeps it, but `:47117` is mac-only
- [ ] Open questions for the user at the end of [windows.md](docs/windows.md); check whether the device wakes a sleeping PC

### Parked

- Like for other players: MediaRemote `LikeTrack` (21) works only with players implementing it (Music.app); Chrome's Media Session has no like
- Custom firmware ([device.md](docs/device.md))
- Wake from sleep: the Mac can't be woken by the device (no HID); not needed
