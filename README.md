# stream

Config-as-code for stream controllers. First device: **Razer Stream Controller** (a rebranded Loupedeck Live),
driven by [Bitfocus Companion](https://github.com/bitfocus/companion).

What the MEDIA page does today (macOS):

- left knob 1 — volume with acceleration, press = mute; left knob 2 — previous/next track, press = play/pause
- left touch strip — current track rotated 90°, turns into an animated volume bar while the volume changes; swipe it to change the volume
- play tile — big ⏸/▶ from the OS playback state
- VK WorkSpace calls — bottom button 2 toggles the microphone (hold = push-to-talk), button 3 the camera, button 4 hangs up;
  LEDs and tiles show the real state from the client, mouse clicks included ([vkcall.md](docs/vkcall.md))
- Yandex Music — hold the play tile or knob 2 to like/unlike the track; 💖/💔 before the track on the strip
  ([ymusic.md](docs/ymusic.md))
- clock tile, haptic tap on presses (and a second one at 400 ms hold), soft haptic ticks while sliding along a strip,
  black tiles without Companion decorations

## Setup (macOS)

1. Close the Loupedeck app. Install Companion: `brew install --cask companion`, open `http://127.0.0.1:8000`.
2. Surfaces → Add Surface Integration → Loupedeck. Settings → Protocols: enable shell command support.
3. System Settings → Privacy & Security → Accessibility → add Companion (and enable `zored_stream` when it asks after step 4).
4. Run:

```bash
make zored_stream-cert
make mediakey zored_stream deploy
make module
make companion
make agent-hooks
```

`make module` clones and builds the [Loupedeck module fork](https://github.com/zored/companion-surface-loupedeck/tree/zored_stream)
next to this repo, points Companion's developer modules path at it, restarts Companion, switches the Loupedeck integration
to the `dev` version and sets the haptics (`HAPTIC`, `HAPTIC_HOLD_MS`, `HAPTIC_SLIDE`, `HAPTIC_STEP`).
`make companion` starts Companion at login with no window (`start_minimised` + a login item).
`make agent-hooks` adds the Claude Code hooks for [agents waiting for me](docs/agents.md) to `~/.claude/settings.json`; rerunning replaces them.

Requirements: Xcode Command Line Tools (`swiftc`, `clang`), Go, Node 22+ with `corepack`, Python 3.

## Layout

| Path | What |
|---|---|
| `tools/gen_pages.py` | Page definitions → `companion/*.companionconfig` |
| `tools/deploy.mjs` | Imports pages into a running Companion over its internal tRPC API |
| `tools/mediakey/` | Media key helper (Swift) |
| `tools/zored_stream/` | one daemon replacing the helpers ([daemon.md](docs/daemon.md)); VK WorkSpace call mic/camera, volume, Now Playing (perl child), Yandex Music like (Accessibility) |
| `tools/icons/` | SVG icons embedded into pages |
| `tools/surface.mjs` | Switches the Loupedeck integration to a module version and sets surface config over tRPC |
| `tools/companion_config.py` | Edits the Companion launcher's `config.json` |
| `tools/sniff.py` | Raw device event sniffer |
| `docs/` | [Companion](docs/companion.md), [macOS helpers](docs/macos.md), [VK WorkSpace calls](docs/vkcall.md), [Yandex Music](docs/ymusic.md), [Windows port](docs/windows.md), [device](docs/device.md) |

## Other ways to drive the device

| Approach | macOS | Windows | Notes |
|---|---|---|---|
| [Loupedeck software](https://loupedeck.com/downloads/) | ✅ | ✅ | Official, Razer support kept in 6.4.1+ |
| [ShoelessTim/RazerStream](https://github.com/ShoelessTim/RazerStream) | ✅ | ❌ | Native Swift app |
| [@loupedeck/node](https://github.com/Julusian/node-loupedeck) | ✅ | ✅ | Node driver, used by Companion |
| [foxxyz/loupedeck](https://github.com/foxxyz/loupedeck) | ✅ | ✅ | Node driver with drawing |
| [LoupixDeck](https://github.com/RadiatorTwo/LoupixDeck) | ❌ | ✅ | .NET app, plugin SDK |

All of them are host-side clients: the device only draws and reports input. No public custom firmware exists; see [device.md](docs/device.md).

Status: [PLAN.md](PLAN.md). Not affiliated with Razer, Loupedeck, Logitech or Bitfocus.
