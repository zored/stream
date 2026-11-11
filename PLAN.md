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
| left knobs `0/0` volume, `1/0` seek (press + turn = tracks, click = play / pause, hold = like), `2/0` chats browse; left strip `0–2/1` track + bar | — |
| tiles `0/2` headphones, `0/3` wi-fi, `0/4` keyboard layout, `0/5` keep-awake, `1/2` play (hold = like, swipes), `1/3` emoji, `1/5` agents waiting, `2/2` mic / chats, `2/3` camera (call) / weather, `2/4` hang up / date + calendar, `2/5` clock + timers | tile `1/4` |
| buttons `3/0` KVM, `3/2` mic / `3/3` camera / `3/4` hang up in a call, `3/4` calendar outside a call, `3/6` dictation; all of `3/0–3/7` in choice mode | buttons `3/1`, `3/5`, `3/7`; `3/2`, `3/3` outside a call |
| right strip `0–2/6` AI limits + model, right knobs `0/7` brightness (hold 3 s = device orientation), `1/7` AI model, `2/7` scroll / Ctrl+Tab | — |

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
  крутилка справа, ряд 1, name on the right strip and tile, port 47124; invariant F21;
  [architecture.html](docs/architecture.html) — layers, pages pipeline, daemon modules, mechanisms, invariants (11 diagrams);
  wi-fi join by clicking the network in Control Center's Wi-Fi details (no password prompts), then Viscosity `Barbos_SE` reconnect
  (`-wifi-vpn`), VPN dot on the tile, hotspot icon, no-internet dot; keep-awake: a tap that turns it off blanked the display
  (idle > `displaysleep`, not a real sleep) → idle reset via `IOPMAssertionDeclareUserActivity` (`4c9311f`);
  KVM module `kvm.go` on 47125 (кнопка 1, `m1ddc`), PIP under discussion; limits: after a Mac sleep the fetch timer stood still,
  the session `resets_at` expired and the bar was cleared → refetch on wake / expiry (`250f73d`); calendar on the clock tile —
  next event/reminder ≤ 12 h under the clock (`●` event, `○` reminder, text rendered to PNG by the daemon via CoreText for emoji),
  touch = 5 s list on крутилка справа, ряд 3 (clock hidden, scroll mode paused), hold / knob press = reminder done ↔ undone (list stays open, `2c5645c`) or
  open the event in Calendar; list moved to the right strip over model + effort, hold opens it, touch acts (`ff5229a`); EventKit, port 47126, F19 rewritten (`15772c9`); wi-fi icon at 60 % (`e574eac`);
  calendar text on a fixed baseline, gray time on the strip (`4628b51`); крутилка справа, ряд 3 in tab mode: press + turn cycles
  the windows of all apps (CGWindowList + AXRaise, no modifier held, `4bfdf34`, F7 updated); keyboard layout on tile `0/2`: small
  flag 🇬🇧/🇷🇺 with a 0.3 s crossfade, press = next layout (TIS on the main-thread run loop, port 47127, `beef447`); emoji picker on
  tile `1/3` ([emoji.md](docs/emoji.md)): tap opens it for 4 s, left strip recent then all, right strip one category, top emoji in a gray
  ring, swipe with inertia (computed in the daemon, the fork only reports touch start/end — fork `54d3abb`), крутилки ряд 1 scroll +
  press types it via `CGEventKeyboardSetUnicodeString`, volume/brightness paused meanwhile; port 47128, F22 (`d043aae`); idle tile =
  a random emoji picked at daemon start (`44e37ce`); no haptic ticks while sliding on a strip — fork `e013f66`, stream `cb9de4e`,
  verified on the device; tab mode press + turn reworked to a real Cmd+Tab (Cmd held until release, Contexts shows the window list,
  F6/F7 updated, `5751ece`); tile layers in the daemon (`layers.go`, F23): date + calendar moved to `2/4` under the call layer,
  clock `2/5` keeps `HH:MM` + timers (tap captures крутилка слева, ряд 3, persisted in `~/.config/zored-stream/timers`, F24, port
  47129, `d42c1a8`); tab mode: a knob press during the 3 s window cycle always wins and gives Cmd+Tab again (`c3f4832`);
  `mediakey` folded into the daemon as `POST :47113/key/<name>`, the CLI, `make mediakey` and `~/.local/bin/mediakey` removed
  (`3d012d5`, `ab4ef3d`); tile swipes on the play tile `1/2`: left/right = prev/next, up/down = like/dislike, touches pinned to the
  start tile so a drag no longer sticks a tile (fork `f0820fa`, stream `2e83202`, verified on the device); perf pass: forks at idle
  9/s → 0 (frontmost via `NSWorkspaceDidActivateApplicationNotification`, Amphetamine via an in-process IOPM check, Viscosity via
  utun), CPU ~17 % → 8–14 % (`448f726`, `45467da`); round 2: one AX walk for choice, a gate for Yandex Music, render keyed by
  frame → ~3 % (`d2699fc`, `a6f761a`)
- 2026-11-04: unread Telegram / VK WorkSpace on tile `2/2` under the call layer: Dock badges via `AXStatusLabel`, banner titles from
  Notification Center via AX, eye icon, press opens the app, port 47130 ([unread.md](docs/unread.md)); docs refreshed against the
  code — `controls.md` layout, `invariants.md` A2/A6/P2 + 8 manual checks, `daemon.md` module table, `README.md` setup (no `swiftc`,
  `m1ddc`, permissions), `macos.md`, `windows.md`, `ymusic.md` (`9180ee6`); note: `zored_wifi.app` was dropped earlier, SSID comes from
  `ipconfig` and the network list from Control Center; кнопка 1 (KVM) is mac-only in the Windows profile; timer knob: 1 min per
  detent, fast turning grows the step 2 → 5 → 10 → 15 → 30 → 60 min (`c43b2a1`, verified live); calendar descriptions: `<br>` =
  newline, other HTML stripped, entities decoded (`3ef3b32`); perf total at idle: CPU ~17 % → ~3 %, forks 9/s → 0, unchanged
  renders ~1000/min → 0

- 2026-11-05: кнопка 5 = calendar (tap = list / action, LED `cal_led`; `1437e17`, `29e911f`); keep-awake blanking fixed twice: idle reset
  before an Amphetamine restart (`b98926f`, `1527646`), then a 0 px mouse nudge on any press because macOS ignores the controller
  (`3a2af70`) — verified on the device; in a call mic / camera / hang-up on кнопки 3 / 4 / 5 under the tiles, LEDs follow the tiles;
  mic / cam tiles white off, red background on; headphones `0/2` ↔ layout `0/4`; the right strip's `3d` / `h:mm` label at `captionSize`
  (`8a161be`…`25fc4d4`); calendar: dark green tile ≤ 5 min before an event, cleared at its end or any action; next event within 7 days,
  the list opens on the shown one (`d0a94a9`); agents: 16 dots, focused chat bigger, slide, title + folder on the right strip per detent,
  detent opens the chat, sessions by activity, child sessions as squares, кнопка 6 freed (`5a3a96b`, `634fed4`, `6885132`); left side
  reworked: крутилка слева, ряд 2 = seek / press + turn tracks / click play / hold like (no second haptic), ряд 3 = chats browse + press,
  кнопка 3 freed (`6e12016`); VK WorkSpace chats: archived (not in the buddylist) → muted mode (`964a181`), a reply shows its own text,
  sender avatar on group chats (`1bccb10`, verified); play tile: pauses < 0.6 s don't flash ⏸ (`08e8d31`); weather on tile `2/3`
  under the call camera: Open-Meteo (no key, no proxy), 😎 for sun, temp + feels-like, tap = forecast slots on крутилка слева, ряд 3,
  port 47131 ([weather.md](docs/weather.md), `eb39c98`, verified on the device); icon set research: Lucide recommended, nothing changed;
  AI.md rule: child sessions ask the user in their own chat, the master never relays
- 2026-11-06: VK WorkSpace calls joined by link (old engine voip3sdk, [vkcall.md](docs/vkcall.md#old-engine-voip3sdk)): state from
  the newest `voip_*.log`, mic / camera via ⇧⌘A / ⇧⌘V, hang-up closes the `In call` window; кнопка 5 hangs up in a call (`fdf7a9c`)
  verified by the user on a live call; on-screen controller panel ([screen.md](docs/screen.md), `tools/screen/`): Swift NSPanel as a
  second Companion Satellite surface in the same group, 12 tiles + 2 strips at the top of the screen, Dock-style magnification, mouse
  click / wheel / right click = tile / knob turn / knob press, strip drags to the daemon (`be6ea38`…`912385d`)
- 2026-11-09: screen panel hidden while the USB controller is plugged, shown when unplugged; after a double-click hide the top edge
  gives a 0.6 s peek of 10 pt, leaving it flies back up (`74da23e`…`27a3f63`, verified by the user); Satellite disconnects for system
  sleep, reconnects on wake (`699805b`); VK WorkSpace chats: 1:1 mute from `getUserInfo` instead of «not in buddylist → muted», so
  unmuted chats are no longer gray (`817e88a`, `6446912`, verified on the device); кнопка 3 toggles the mic / push-to-talk in a call
  again, broken by `6e12016` (`5360b2c`, verified on the device); voip3sdk: a mic tap sent ⇧⌘A twice (press held a lock while the release waited), now one toggle, hold = push-to-talk
  (`3978343`, verified on the device)
- 2026-11-10: usage: red dot fixed, it stays red only while the network / VPN is really down and clears ~1 min after (`a55a688`,
  verified on the device); [logic.md](docs/logic.md) — behaviour spec of every control and daemon module for the Windows port by
  Codex (`738b917`); daemon binary kept out of git via `.gitignore` (`e0e5741`)
- 2026-11-11: chats browse reads messages: крутилка справа, ряд 2 steps unread messages of the chat (mini avatar in group chats),
  ряд 3 scrolls a long one smoothly; hold 400 ms on the chats tile or крутилка слева, ряд 3 marks the chat read (Telegram
  `messages.readHistory`, VK WorkSpace `rapi/setDlgState`) (`6a1581e`, `3af6057`, verified on the device); device orientation for a
  controller turned on its side: hold крутилка справа, ряд 1 3 s → 90 → 0 → 270 → 180, only in the module fork (pictures and touches
  turned before the device, the upside-down strip's cells 180°, file `~/.config/zored-stream/orientation`, default 90), the daemon
  reads it only for `strip_flip` (left-strip bars) and upright emoji glyphs; Companion's Surface Rotation stays off (it remaps the
  grid); invariant A12 (fork `bf0ca30`, stream `05de107`…`1e2eff4`, verified on the device)

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
- [ ] Tab mode: press + turn крутилка справа, ряд 3 cycles windows (checked over HTTP only)
- [ ] Emoji picker on the device: look, swipe + inertia, typing into a field, haptics (checked over the API only)
- [ ] Unread tile on the device; choice cards and Yandex Music like after the perf round 2 (not run live)
- [ ] Date tile on the device; the 00:05 alarm seen once = a turn below 0 at the clock tile (assumed)
- [ ] Screen panel after a real sleep: `screen.log` shows `sleep` / `wake`, no `Satellite closed`
- [ ] Muted 1:1 VK WorkSpace chat shows gray on the chats tile (test + API only)
- [ ] Emoji picker turned: knob direction inverted while the device is turned + left strip mirrored (committed 11 ноя on the user's «сохраняй», not seen on the device)
- [ ] Orientation 0° / 180° / 270° on the device (90° is the one used daily)
- [ ] 2026-11-05 batch on the device: green calendar before an event + list opening on the shown event; agents 16 dots / slide / hollow dot for a chat not on the tile; крутилка слева, ряд 2 (tracks, like) and ряд 3 (chats); jirabot in the gray (muted) mode; the `3d` / `h:mm` label size; chats animation smoothness

### More buttons and actions

New features go on the MEDIA page as buttons; separate pages only once it runs out of room.

- [ ] Now Playing: a race between two publishes fixed (`ea9b8fd`), the stale-track symptom itself never reproduced — reopen if seen again
- [ ] Icon set — waiting for the user's pick (research 2026-11-04, nothing changed): today 9 SVGs in `tools/icons/` (grid 48, stroke 3.5) + ~11 inline in the daemon; candidates Lucide (ISC) / Tabler (MIT) / Phosphor (MIT), SF Symbols out by license; recommendation Lucide, stroke 1.75 on 24, 60 px in a 90 px tile, vendored in `tools/icons/lucide/` with a pinned version + `go:embed`; off state = same icon in gray; migration: call tiles → media → wifi/headphones/sun → timers/agents/chats/layout — waiting for the user's pick
- [ ] IDE / terminal pages; page switching by frontmost app (`zored_stream` already reads it via `lsappinfo` and can push a variable)
- [ ] Integrations worth a look: OBS, Zoom, GitHub/CI status via HTTP

### Ideas (mechanisms assumed, not checked)

Desk and display:
- [ ] Monitor brightness: DDC/CI via `m1ddc` or BetterDisplay CLI. PIP dropped (2026-11-04, user's call); KVM on кнопка 1 is done (`kvm.go`)
- [ ] Display off (`pmset displaysleepnow`), lock
- [ ] Perf round 4: after round 3 (CoreAudio listeners, `5b8a196`) idle is still ~3 %; next by CPU: dictation AX search in the Claude window (46 %), choice (10 %), agents scan (8 %), Yandex Music AX (5 %); log spam `ymusic: player bar not found` every 3 s since 11-02; stale `np_*` on a natural track change not reproduced yet (manual next = 1–1.5 s); a bare strip tap never reaches the daemon (fork hook, not done)

Windows and apps:
- [ ] Frontmost app icon on a tile, press cycles its windows
- [ ] Tile swipes on more tiles (play tile done, see Done). Research (2026-11-03, sniffed: one touch surface, ~70 move events/s, see [device.md](docs/device.md#touch)): `@loupedeck/core` `onTouch` delivers `touchmove` for the center
  with per-tile x/y and multitouch ids, the fork's `instance.ts` drops it (`continue`); patch = `src/tile-swipe.ts` beside
  `strip-drag.ts`: remember x/y on `touchstart`, on `touchend` with ≥ 40 px and ≤ 400 ms post `/tile/swipe?from=<id>&dir=l|r|u|d`;
  fix first: the fork recomputes `touch.target.control` per event, so a drag from tile A to B sends `keyDown(A)` + `keyUp(B)` and A
  sticks, release in a gap sends no `keyUp` — keep the control from `touchstart`. Conflict with tap/hold: `keyDown` goes out at once,
  so swipes suit actions that need no hold: pages left/right, track prev/next, drag between tiles for volume/brightness

AI agents:
- [ ] Codex sessions on the agents tile ([agents.md](docs/agents.md#next-codex))
- [ ] Haptic on a new waiting session (needs a fork endpoint)

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
