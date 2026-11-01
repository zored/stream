# Plan

## 1. Loupedeck software, baseline

- [x] Install Loupedeck 6.4.1+ (first version that kept Razer support after the reversal): `6.4.1.364`, service `LPS:6.4.1.3246`
- [x] Device seen over USB: `Razer Stream Controller`, VID `0x1532` PID `0x0D06`, `/dev/cu.usbmodem1101`; the app calls it `Loupedeck40`
- [x] Firmware: `0.2.14`, "up to date" per the app
- [ ] Sync current Loupedeck profiles into `profiles/loupedeck/` as a reference for the Companion rebuild
- [ ] List what the official app can't do → requirements for phase 2

## 2. Bitfocus Companion as the host client

Custom firmware isn't needed for own buttons: the device only draws tiles and reports input,
all behaviour lives in the host client. Decision: Companion only, no own client.
Only one process can hold the serial port: never run Companion together with Loupedeck.

Facts:

- Companion's Loupedeck surface ([companion-surface-loupedeck](https://github.com/bitfocus/companion-surface-loupedeck)) is built on `@loupedeck/node` ^2.2.2
- Companion docs (v4.2): RSC supported, firmware ≥ v0.2.0, **Loupedeck software must not be running**
- Since 4.3 surfaces are modules: Surfaces page → Add Surface Integration → Loupedeck. Current docs: v5.1
- macOS: `brew install --cask companion` → 5.1.1 (macOS 13+)
- Touch strips can be split into 6 controls with the community [split-strips surface](https://github.com/ninoleto/companion-surface-loupedeck-split-strips)
- Loupedeck's service autostarts via launch agent `com.logi.pluginservice.launch` (`RunAtLoad`, no `KeepAlive`)
- Grid: 4 rows × 8 columns (from `@loupedeck/node` model `live.ts`), `row/column`:
  - col 0, rows 0–2: left knobs · col 1: left touch strip (3 cells)
  - cols 2–5, rows 0–2: the 12 touch tiles (80×80 px)
  - col 6: right touch strip · col 7, rows 0–2: right knobs
  - row 3, cols 0–7: the 8 RGB buttons (LED = button background colour)
- Companion 5.1 buttons are layered. The legacy `POST /api/location/<p>/<r>/<c>/style` only works on buttons with `options.canModifyStyleInApis: true`
- REST API v2 (`/api/v2`, bearer key, Settings → Protocols) covers only connections and surfaces in 5.1.1 — no buttons
- Config as code: `GET /int/export/page/<n>?format=json|yaml` (no auth) exports a page; import is UI-only (Import / Export). Format: `version: 12`, `page.controls[row][col]`
- Debug log: `~/Library/Application Support/companion/logs/`; internal action `custom_log` writes to it

Known weak spots to check early: keystrokes on macOS need a helper; per-app page switching may need a module or script.

### 2a. Bring-up on macOS

- [x] Install Companion 5.x: `brew install --cask companion` → 5.1.1; first run asks for interface/port, then web UI at `http://127.0.0.1:8000`
- [x] Free the device: quitting the Loupedeck app also stops `LogiPluginService`. Roll back: `launchctl kickstart gui/$(id -u)/com.logi.pluginservice.launch` and open Loupedeck
- [ ] Disable Loupedeck autostart once Companion is the daily driver: `launchctl disable gui/$(id -u)/com.logi.pluginservice.launch` (undo: `enable`)
- [x] Surfaces → Add Surface Integration → Loupedeck v1.1.1 (store module; split-strips isn't in the store). RSC detected as `loupedeck:<serial>`
- [x] Input reaches Companion: tiles, knob press, bottom buttons show up in the debug log as `Button <page>/<row>/<col> pressed`
  - Raw device events checked with `tools/sniff.py` (quit Companion first): press `0x00 <id> 00|01`, rotate `0x01 <id> ±1`, touch `0x4d`/`0x6d`.
    IDs: buttons `0x07..0x0e`, left knobs `0x01..0x03`, right knobs `0x04..0x06`
  - Firmware 0.2.14 echoes the handshake back as `0x73` frames; Companion logs them as "unhandled incoming message", harmless
  - "No reaction" on knobs / buttons / strips = nothing configured there: no actions, LED = black background, no buttons in strip columns. Page nav doesn't wrap past the last page
- [x] Full input check with a generated test page (`tools/gen_test_page.py` → `companion/test-page.companionconfig`, import via Import / Export):
  all 12 tiles, 8 buttons, 6 knobs (press + left + right), 6 strip cells log `TEST <control> <event>`
- [x] Button LEDs show the background colours of the test page (rainbow verified)
- [ ] Try the split-strips surface; keep whichever maps better
- [ ] Autostart Companion at login, headless (no window)

### 2b. Actions

- [ ] Find how to send keystrokes on macOS (candidates: VICREO Hotkey module + listener, internal shell action with `osascript`)
- [ ] Shell / open app / open URL via internal actions
- [ ] Media + volume, display brightness
- [ ] Knobs: volume, scroll, brightness; button LEDs reflect state (mute, recording, etc.)
- [ ] Pages: IDE, browser, terminal, calls; check whether page switching by frontmost app is possible (module or small helper script)
- [ ] Integrations worth a look: OBS, Zoom, Spotify, GitHub/CI status via HTTP

### 2c. Config as code

- [ ] Export Companion config into `companion/` (full export), commit after every meaningful change
- [ ] Document restore: fresh install → import → works
- [ ] Windows: install Companion, import the same export, note OS-specific actions (keystrokes, shell)

Not planned: own client on `@loupedeck/node` / `foxxyz/loupedeck`, [LoupixDeck](https://github.com/RadiatorTwo/LoupixDeck) (no macOS),
[ShoelessTim/RazerStream](https://github.com/ShoelessTim/RazerStream) (no Windows).

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
