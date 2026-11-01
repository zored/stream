# Plan

## 1. Loupedeck software, configured properly

- [ ] Install Loupedeck 6.4.1+ (first version that kept Razer support after the reversal)
- [ ] Check firmware version shown by the app, write it down (protocol notes are for 0.2.8)
- [ ] Profiles per app: IDE, browser, terminal, calls
- [ ] Pages + knob actions: volume, scroll, brightness
- [ ] Shell / AppleScript actions for dev workflows
- [ ] Export profiles into `profiles/` so they are versioned
- [ ] List what the official app can't do → requirements for phase 2

## 2. Own buttons on macOS and Windows

Original idea: custom firmware. Reality check:

- ShoelessTim/RazerStream is a macOS host app, not firmware.
- No public custom firmware found for Loupedeck Live / RSC (assumed, search only).
- Host-side drivers already give full control: tiles, knobs, buttons, LEDs, haptics.

So the main track is a cross-platform host app; firmware is a research side track.

### 2a. Host app (main track)

- [ ] Find out why RazerStream doesn't build here. Suspect: only Command Line Tools, no Xcode (`xcodebuild` missing). Read the CI workflow to see what toolchain it uses
- [ ] Pick a stack for macOS + Windows:
  - [@loupedeck/node](https://github.com/Julusian/node-loupedeck) — one codebase, both OSes
  - [Bitfocus Companion](https://github.com/bitfocus/companion) — maybe enough without writing code
  - RazerStream on macOS + something else on Windows — two codebases, avoid
- [ ] Spike: connect, draw a tile, read a knob, set a button LED on both OSes
- [ ] Config format (YAML/JSON in repo), actions: keystroke, shell, open URL/app, media
- [ ] Per-app page switching (frontmost app)

### 2b. Firmware (research side track)

- [ ] Identify the MCU (teardown photos, FCC ID filings)
- [ ] Look for a debug port (SWD/JTAG) and how the official app updates firmware
- [ ] Decide go/no-go only after that. Bricking risk: never flash without a recovery path

## 3. GitHub Actions for knowledge sharing

- [ ] CI: build host app on `macos-latest` and `windows-latest`
- [ ] Releases: attach built binaries on tag
- [ ] Publish protocol and setup notes (GitHub Pages or just markdown in repo)
- [ ] If RazerStream is forked: build it in CI here, so "doesn't compile locally" never blocks
