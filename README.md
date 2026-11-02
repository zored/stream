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
- clock tile, haptic tap on presses (and a second one at 400 ms hold), black tiles without Companion decorations

## Setup (macOS)

1. Close the Loupedeck app. Install Companion: `brew install --cask companion`, open `http://127.0.0.1:8000`.
2. Surfaces → Add Surface Integration → Loupedeck. Settings → Protocols: enable shell command support.
3. System Settings → Privacy & Security → Accessibility → add Companion (and `~/.local/bin/ymusic` after step 4).
4. Run:

```bash
make mediakey streamd vkcall ymusic deploy
make patch-module
```

5. Restart Companion.

Requirements: Xcode Command Line Tools (`swiftc`, `clang`), Go, Node 22+, Python 3.

## Layout

| Path | What |
|---|---|
| `tools/gen_pages.py` | Page definitions → `companion/*.companionconfig` |
| `tools/deploy.mjs` | Imports pages into a running Companion over its internal tRPC API |
| `tools/mediakey/` | Media key helper (Swift) |
| `tools/streamd/` | Volume + Now Playing daemon (Objective-C, hosted by `/usr/bin/perl`) |
| `tools/vkcall/` | VK WorkSpace call mic/camera daemon (Go, the client's local gRPC) |
| `tools/ymusic/` | Yandex Music like/dislike daemon (Swift, Accessibility) |
| `tools/icons/` | SVG icons embedded into pages |
| `tools/patch_module.py` | Adds haptics and strip swipe to the Companion Loupedeck module |
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
