# Plan

## 1. Loupedeck software, baseline

- [x] Install Loupedeck 6.4.1+ (first version that kept Razer support after the reversal): `6.4.1.364`, service `LPS:6.4.1.3246`
- [x] Device seen over USB: `Razer Stream Controller`, VID `0x1532` PID `0x0D06`, `/dev/cu.usbmodem1101`; the app calls it `Loupedeck40`
- [x] Firmware: `0.2.14`, "up to date" per the app
- [ ] Sync current Loupedeck profiles into `profiles/loupedeck/` as a reference for the Companion rebuild
- [ ] List what the official app can't do → requirements for phase 2

## 2. Bitfocus Companion as the host client

Custom firmware isn't needed for own buttons: the device only draws tiles and reports input,
all behaviour lives in the host client. Companion is the main choice: macOS + Windows, open source,
big module ecosystem, config is exportable.

Facts:

- Companion docs (v4.2): RSC supported, firmware ≥ v0.2.0, **Loupedeck software must not be running**
- Since 4.3 surfaces are modules: Surfaces page → Add Surface Integration → Loupedeck. Current docs: v5.1
- macOS: `brew install --cask companion` was 4.2.x at last check; 5.x from [GitHub releases](https://github.com/bitfocus/companion/releases) (macOS 13.5+)
- Touch strips can be split into 6 controls with the community [split-strips surface](https://github.com/ninoleto/companion-surface-loupedeck-split-strips)

Other clients, if Companion falls short:

| Client | macOS | Windows | What it is |
|---|---|---|---|
| [LoupixDeck](https://github.com/RadiatorTwo/LoupixDeck) | ❌ | ✅ | .NET app for Loupedeck/RSC with a plugin SDK; Linux too |
| [ShoelessTim/RazerStream](https://github.com/ShoelessTim/RazerStream) | ✅ | ❌ | Native Swift app, imports Loupedeck profiles |
| Own client on [@loupedeck/node](https://github.com/Julusian/node-loupedeck) | ✅ | ✅ | Full control, ours to maintain |

### 2a. Bring-up on macOS

- [ ] Install Companion 5.x
- [ ] Free the device: quit Loupedeck, stop `LogiPluginService`, disable its login item / launch agent (write down how to roll back)
- [ ] Add the Loupedeck surface integration, rescan USB, check all 12 tiles, 6 knobs (turn + press), 8 buttons + LEDs
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

### 2d. Fallback

- [ ] RazerStream build: suspect only Command Line Tools, no Xcode (`xcodebuild` missing)
- [ ] Own client on @loupedeck/node only if Companion can't cover a must-have

## 3. Wake the PC from the controller

Facts (verified via `ioreg`): the device exposes CDC serial + vendor + audio interfaces, **no HID**.
So it can't send a keyboard "wake" like a normal keyboard. Whether it sets USB remote wakeup at all is unknown.

- [ ] Test: put the machine to sleep, press a button / turn a knob. macOS and Windows. Write down the result
- [ ] Windows: Device Manager → device → Power Management → "Allow this device to wake the computer", if present
- [ ] Plan A — don't sleep, only turn off the display: system stays awake, Companion dims the deck and on any press wakes the display
  (`caffeinate -u` on macOS, `SetThreadExecutionState` on Windows). Cost: power draw
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
