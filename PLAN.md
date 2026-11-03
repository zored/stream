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
| tiles `0/3` wi-fi, `0/4` headphones, `0/5` keep-awake, `1/2` play (hold = like), `1/5` agents waiting, `2/2` mic, `2/3` camera, `2/4` hang up, `2/5` clock + calendar | tiles `0/2`, `1/3–1/4` |
| buttons `3/0` KVM (in progress), `3/1` mic, `3/2` camera, `3/3` hang up, `3/5` agents, `3/6` dictation; all of `3/0–3/7` in choice mode | buttons `3/4`, `3/7` (outside choice mode) |
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
  ([windows.md](docs/windows.md)); CI: concurrency + timeout, pages ↔ daemon contract check, Go tests; [invariants.md](docs/invariants.md);
  verified by the user: calls reconnect after a VK WorkSpace restart, seek + cover with Chrome, keep-awake hold → sleep;
  headphones tile `0/4` — Accentum BT toggle, output icon (speaker/headphones, yellow while connecting, red on error), KVM auto route
  Logi + USB camera, port 47123 ([macos.md](docs/macos.md)); wi-fi tile `0/3` — hold toggles power, touch = 5 s network pick on
  крутилка справа, ряд 1, name on the right strip and tile, scan via `zored_wifi.app` (Location permission), port 47124; invariant F21;
  [architecture.html](docs/architecture.html) — layers, pages pipeline, daemon modules, mechanisms, invariants (11 diagrams);
  wi-fi join by clicking the network in Control Center's Wi-Fi details (no password prompts), then Viscosity `Barbos_SE` reconnect
  (`-wifi-vpn`), VPN dot on the tile, hotspot icon, no-internet dot; keep-awake: a tap that turns it off blanked the display
  (idle > `displaysleep`, not a real sleep) → idle reset via `IOPMAssertionDeclareUserActivity` (`4c9311f`);
  KVM module `kvm.go` on 47125 (кнопка 1, `m1ddc`), PIP under discussion; limits: after a Mac sleep the fetch timer stood still,
  the session `resets_at` expired and the bar was cleared → refetch on wake / expiry (`250f73d`); calendar on the clock tile —
  next event/reminder ≤ 12 h under the clock (`●` event, `○` reminder, text rendered to PNG by the daemon via CoreText for emoji),
  touch = 5 s list on крутилка справа, ряд 3 (clock hidden, scroll mode paused), hold / knob press = reminder done ↔ undone (list stays open, `2c5645c`) or
  open the event in Calendar; list moved to the right strip over model + effort, hold opens it, touch acts (`ff5229a`); EventKit, port 47126, F19 rewritten (`15772c9`); wi-fi icon at 60 % (`e574eac`)

## Next

### Verify

- [ ] Restore test from scratch: fresh Companion → README setup → works — will be done on the Windows PC
- [ ] Choice mode: CLI permission prompt in iTerm2 (кнопка 1 = Esc not run)
- [ ] Seek and cover with Music.app; dictation resume with a browser player
- [ ] Right strip slide picks the model on the device
- [ ] Headphones and wi-fi tiles on the device: icons, yellow/red states, the `›` arrow and the network name on the strip
- [ ] Wi-fi switch end to end from the device (`bo phone` ↔ `zored_5G`): dot red → yellow → white, VPN back
- [ ] Keep-awake: on, 15 min without keyboard/mouse, short tap → the display must stay on
- [ ] Calendar tile on the device: emoji, wrapping, list on the knob; hold on an event opens it in Calendar (not run)

### More buttons and actions

New features go on the MEDIA page as buttons; separate pages only once it runs out of room.

- [ ] Mic, camera and hang-up tiles do something else outside a call
- [ ] IDE / terminal pages; page switching by frontmost app (`zored_stream` already reads it via `lsappinfo` and can push a variable)
- [ ] Integrations worth a look: OBS, Zoom, GitHub/CI status via HTTP
- [ ] `mediakey` into the daemon as `POST /key/<name>` (drops the `exec` and Companion's Accessibility; Windows needs the route anyway)

### Ideas (mechanisms assumed, not checked)

Desk and display:
- [ ] In progress (child session): KVM switch to Windows on кнопка 1 — `m1ddc set input 15` (MSI 321URX, from `~/switch_to_windows.command`)
- [ ] Monitor brightness, PiP: DDC/CI via `m1ddc` or BetterDisplay CLI
- [ ] Display off (`pmset displaysleepnow`), lock
- [ ] Performance pass over `zored_stream` and helpers: measure CPU/wakeups, replace polling with CoreAudio/notification listeners,
  cut redundant Companion variable sets

Windows and apps:
- [ ] App switcher (Cmd+Tab) on a knob, same release rules as Ctrl+Tab ([invariants.md](docs/invariants.md) S1)
- [ ] Timers and alarm on one knob: turn sets a duration or a wall-clock time (press+turn toggles), press starts;
  countdown / alarm time on the strip, gray dot on the clock tile while armed, haptic + sound at the end; owned by `zored_stream`
- [ ] Keyboard layout on a tile (EN/RU), press switches: Text Input Sources (`TISCopyCurrentKeyboardInputSource`, `TISSelectInputSource`,
  change notification) in `zored_stream`
- [ ] Emoji picker: knob scrolls a short favorites list on the strip, press types the emoji into the focused field
  (`CGEventKeyboardSetUnicodeString` in `zored_stream`, no clipboard)
- [ ] Frontmost app icon on a tile, press cycles its windows
- [ ] Tile swipes (researched 2026-11-03, sniffed: one touch surface, ~70 move events/s, see [device.md](docs/device.md#touch)): `@loupedeck/core` `onTouch` delivers `touchmove` for the center
  with per-tile x/y and multitouch ids, the fork's `instance.ts` drops it (`continue`); patch = `src/tile-swipe.ts` beside
  `strip-drag.ts`: remember x/y on `touchstart`, on `touchend` with ≥ 40 px and ≤ 400 ms post `/tile/swipe?from=<id>&dir=l|r|u|d`;
  fix first: the fork recomputes `touch.target.control` per event, so a drag from tile A to B sends `keyDown(A)` + `keyUp(B)` and A
  sticks, release in a gap sends no `keyUp` — keep the control from `touchstart`. Conflict with tap/hold: `keyDown` goes out at once,
  so swipes suit actions that need no hold: pages left/right, track prev/next, drag between tiles for volume/brightness

AI agents:
- [ ] Codex sessions on the agents tile ([agents.md](docs/agents.md#next-codex))
- [ ] Haptic on a new waiting session (needs a fork endpoint)

Communication:
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
