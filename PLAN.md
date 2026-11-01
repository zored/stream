# Plan

## 1. Loupedeck software, configured properly

- [x] Install Loupedeck 6.4.1+ (first version that kept Razer support after the reversal): `6.4.1.364`, service `LPS:6.4.1.3246`
- [x] Device seen over USB: `Razer Stream Controller`, VID `0x1532` PID `0x0D06`, `/dev/cu.usbmodem1101`; the app calls it `Loupedeck40`
- [ ] Check firmware version shown by the app, write it down (protocol notes are for 0.2.8). The app downloaded `FW_C40_T0_0.2.14.bin`: find out whether it was flashed
- [ ] Profiles per app: IDE, browser, terminal, calls
- [ ] Pages + knob actions: volume, scroll, brightness
- [ ] Shell / AppleScript actions for dev workflows
- [ ] Sync profiles into `profiles/` so they are versioned
- [ ] List what the official app can't do → requirements for phase 2

## 2. Own host client on macOS and Windows

Custom firmware isn't needed for own buttons: the device is a dumb terminal (draw tiles, report presses),
all behaviour lives in the host client. No public custom firmware exists anyway.

Candidates:

| Client | macOS | Windows | What it is |
|---|---|---|---|
| [Bitfocus Companion](https://github.com/bitfocus/companion) | ✅ | ✅ | Most mature. Buttons → HTTP/OSC/MIDI/shell via modules, web UI config. [RSC surface](https://github.com/ninoleto/companion-surface-loupedeck-split-strips) |
| [LoupixDeck](https://github.com/RadiatorTwo/LoupixDeck/wiki) | ❌ | ✅ | .NET app for Loupedeck/RSC with a plugin SDK; Linux too |
| [ShoelessTim/RazerStream](https://github.com/ShoelessTim/RazerStream) | ✅ | ❌ | Native Swift app, imports Loupedeck profiles |
| Own client on [@loupedeck/node](https://github.com/Julusian/node-loupedeck) | ✅ | ✅ | Full control, config in repo, ours to maintain |

- [ ] Companion first: quit `LogiPluginService` (it holds the port), connect RSC, rebuild a few Loupedeck pages. Note what's missing
- [ ] Find out why RazerStream doesn't build here. Suspect: only Command Line Tools, no Xcode (`xcodebuild` missing)
- [ ] If Companion falls short → own client on @loupedeck/node:
  - [ ] Spike: connect, draw a tile, read a knob, set a button LED on both OSes
  - [ ] Config format (YAML in repo), actions: keystroke, shell, open URL/app, media
  - [ ] Per-app page switching (frontmost app)
  - [ ] Autostart: launchd agent (macOS), scheduled task / service (Windows)

## 3. Wake the PC from the controller

Facts (verified via `ioreg`): the device exposes CDC serial + vendor + audio interfaces, **no HID**.
So it can't send a keyboard "wake" like a normal keyboard. Whether it sets USB remote wakeup at all is unknown.

- [ ] Test: put the machine to sleep, press a button / turn a knob. macOS and Windows. Write down the result
- [ ] Windows: Device Manager → device → Power Management → "Allow this device to wake the computer", if present
- [ ] Plan A — don't sleep, only turn off the display: system stays awake, client dims the deck and on any press wakes the display
  (`caffeinate -u` on macOS, `SetThreadExecutionState` on Windows). Cost: power draw
- [ ] Plan B — companion HID: tiny USB board (RP2040 / Pro Micro, QMK/KMK) with one key that sends a wake keystroke
- [ ] Plan C — Wake-on-LAN from phone / another box (Windows desktop mainly; Mac: "Wake for network access")
- [ ] Plan D — firmware that adds a HID interface with remote wakeup: only if the firmware track below gets a go

## 4. Firmware (research side track)

- [ ] Identify the MCU (teardown photos, FCC ID filings)
- [ ] Inspect the vendor blob `FW_C40_T0_0.2.14.bin` locally (don't commit): header, MCU arch, update protocol
- [ ] Look for a debug port (SWD/JTAG) and a recovery path
- [ ] Go/no-go only after that. Never flash without a recovery path

## 5. GitHub Actions for knowledge sharing

- [ ] CI: build host client on `macos-latest` and `windows-latest`
- [ ] Releases: attach built binaries on tag
- [ ] Publish protocol and setup notes (GitHub Pages or just markdown in repo)
- [ ] If RazerStream is forked: build it in CI here, so "doesn't compile locally" never blocks
