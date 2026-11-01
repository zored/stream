# Plan

## 1. Loupedeck software, baseline

- [x] Install Loupedeck 6.4.1+ (first version that kept Razer support after the reversal): `6.4.1.364`, service `LPS:6.4.1.3246`
- [x] Device seen over USB: `Razer Stream Controller`, VID `0x1532` PID `0x0D06`, `/dev/cu.usbmodem1101`; the app calls it `Loupedeck40`
- [x] Firmware: `0.2.14`, "up to date" per the app
- [ ] Sync current Loupedeck profiles into `profiles/loupedeck/` as a reference for the Companion rebuild
- [ ] List what the official app can't do → requirements for phase 2

## 2. Host client: Companion vs own Node client

Custom firmware isn't needed for own buttons: the device only draws tiles and reports input,
all behaviour lives in the host client. Try both tracks, then pick one.
Only one process can hold the serial port: run them one at a time, never together with Loupedeck.

| | Bitfocus Companion | Own client on `@loupedeck/node` | Own client on `foxxyz/loupedeck` |
|---|---|---|---|
| What | Ready app, web UI, hundreds of modules | Device library, we write the app | Device library, we write the app |
| Device driver | `@loupedeck/node` ^2.2.2 (via [companion-surface-loupedeck](https://github.com/bitfocus/companion-surface-loupedeck)) | same | own implementation |
| Maintainer | Bitfocus, ⭐2.3k | Julusian (Companion dev), ⭐11 but battle-tested inside Companion | foxxyz, ⭐140, v7.1.0 (2026-10-29) |
| Drawing | Built-in button styles, text, PNG | Raw pixel buffers, bring own renderer | Built-in via `canvas` (native dep) |
| Config | UI, export to file | Ours: YAML in repo | Ours: YAML in repo |
| Keystrokes on macOS | Needs a helper (VICREO / shell) | Ours: shell / native module | Ours: shell / native module |
| Per-app pages | Unclear, needs a module or script | Ours | Ours |
| Cost | Zero code, generic UX | Code + maintenance, full control | Code + maintenance, full control |

Node library pick: `@loupedeck/node` — same driver as Companion, so protocol fixes land in both;
`foxxyz/loupedeck` only if its built-in drawing saves real time.

Facts:

- Companion docs (v4.2): RSC supported, firmware ≥ v0.2.0, **Loupedeck software must not be running**
- Since 4.3 surfaces are modules: Surfaces page → Add Surface Integration → Loupedeck. Current docs: v5.1
- macOS: `brew install --cask companion` was 4.2.x at last check; 5.x from [GitHub releases](https://github.com/bitfocus/companion/releases) (macOS 13.5+)
- Touch strips can be split into 6 controls with the community [split-strips surface](https://github.com/ninoleto/companion-surface-loupedeck-split-strips)

Other ready apps, not planned: [LoupixDeck](https://github.com/RadiatorTwo/LoupixDeck) (no macOS),
[ShoelessTim/RazerStream](https://github.com/ShoelessTim/RazerStream) (no Windows).

### 2a. Companion bring-up on macOS

- [ ] Install Companion 5.x
- [ ] Free the device: quit Loupedeck, stop `LogiPluginService`, disable its login item / launch agent (write down how to roll back)
- [ ] Add the Loupedeck surface integration, rescan USB, check all 12 tiles, 6 knobs (turn + press), 8 buttons + LEDs
- [ ] Try the split-strips surface; keep whichever maps better
- [ ] Autostart Companion at login, headless (no window)

### 2b. Companion actions

- [ ] Find how to send keystrokes on macOS (candidates: VICREO Hotkey module + listener, internal shell action with `osascript`)
- [ ] Shell / open app / open URL via internal actions
- [ ] Media + volume, display brightness
- [ ] Knobs: volume, scroll, brightness; button LEDs reflect state (mute, recording, etc.)
- [ ] Pages: IDE, browser, terminal, calls; check whether page switching by frontmost app is possible (module or small helper script)
- [ ] Integrations worth a look: OBS, Zoom, Spotify, GitHub/CI status via HTTP

### 2c. Companion config as code

- [ ] Export Companion config into `companion/` (full export), commit after every meaningful change
- [ ] Document restore: fresh install → import → works
- [ ] Windows: install Companion, import the same export, note OS-specific actions (keystrokes, shell)

### 2d. Own Node client spike (`client/`)

- [ ] TypeScript + `@loupedeck/node`, Node 22 (same as Companion's surface)
- [ ] Connect, draw a tile (text + PNG), read knob turn/press, touch, set button LED
- [ ] YAML config: pages → controls → actions (keystroke, shell, open URL/app, media)
- [ ] Per-app page switching (frontmost app: `NSWorkspace` via a tiny helper on macOS, `GetForegroundWindow` on Windows)
- [ ] Autostart: launchd agent (macOS), scheduled task (Windows)
- [ ] Same spike on Windows

### 2e. Decide

Rebuild the same 2–3 pages in both, then compare:

- [ ] Time to add a new button / page
- [ ] Must-haves covered: keystrokes, per-app pages, LED state, wake handling (phase 3)
- [ ] Latency and stability over a week of daily use
- [ ] Windows parity

Pick one, archive the other track's notes here.

## 3. Wake the PC from the controller

Facts (verified via `ioreg`): the device exposes CDC serial + vendor + audio interfaces, **no HID**.
So it can't send a keyboard "wake" like a normal keyboard. Whether it sets USB remote wakeup at all is unknown.

- [x] Test on MacBook: asleep, status button stays blue (USB power kept), presses **don't wake** it
- [ ] Same test on Windows
- [ ] Windows: Device Manager → device → Power Management → "Allow this device to wake the computer", if present
- [ ] Plan A — don't sleep, only turn off the display: system stays awake, the client dims the deck and on any press wakes the display
  (`caffeinate -u` on macOS, `SetThreadExecutionState` on Windows). Cost: power draw; on a MacBook only sensible on AC / clamshell
- [ ] Plan B — companion HID: tiny USB board (RP2040 / Pro Micro, QMK/KMK) with one key that sends a wake keystroke
- [ ] Plan C — Wake-on-LAN from phone / another box (Windows desktop mainly; Mac: "Wake for network access")

## 4. Custom firmware — parked

Research done 2026-11-01:

- No public custom firmware, firmware dump or MCU teardown found for Loupedeck Live / RSC (web search; GitHub only has host-side drivers)
- Vendor blob `FW_C40_T0_0.2.14.bin` (397,940 bytes): entropy 8.0 bits/byte, no ARM vector table, no readable strings → encrypted (or compressed + encrypted). Patching it isn't possible without the key
- History: firmware 1.x exposed USB networking + HTTP/WebSocket; 0.2.x is serial + WebSocket ([scottlaird/loupedeck](https://pkg.go.dev/github.com/scottlaird/loupedeck))
- Only remaining path: open the device, identify the MCU, try SWD; likely read-protected. High effort, bricking risk

Verdict: no-go for now. Revisit only if a must-have (e.g. HID wake) can't be solved on the host side.

## 5. GitHub Actions for knowledge sharing

- [ ] Validate `companion/` export on push (valid JSON, no secrets)
- [ ] Publish setup notes (GitHub Pages or plain markdown)
- [ ] If an own client appears: build on `macos-latest` + `windows-latest`, attach binaries on tag
