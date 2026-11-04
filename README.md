# stream

Config-as-code for stream controllers. First device: **Razer Stream Controller** (a rebranded Loupedeck Live),
driven by [Bitfocus Companion](https://github.com/bitfocus/companion).

What the MEDIA page does today (macOS), every control in detail: [docs/controls.md](docs/controls.md).
Rules that must keep holding: [docs/invariants.md](docs/invariants.md).

- left knobs: volume (press = mute), previous / next track (press = play/pause, hold = Yandex Music like), seek
- left strip: the current track rotated 90° with a progress bar tinted by the cover; a bar for volume, brightness and seek; swipe = volume
- tiles: keyboard layout, wi-fi (+ Viscosity), headphones, keep-awake (hold = sleep), track cover with ▶/⏸ and swipes (prev / next, like / dislike), emoji picker, Claude Code agents waiting, VK WorkSpace mic / camera, day + calendar (hang-up during a call), clock + timers
- buttons: KVM to the Windows PC; VK WorkSpace mic (hold = push-to-talk), camera, hang-up; agents; dictation into Claude Desktop;
  in choice mode кнопки 1–8 answer Claude's question, permission and next-step cards
- right strip: Claude usage limits + the model and effort; right knobs: Mac / controller brightness, Claude Desktop model + effort,
  mouse wheel or Ctrl+Tab / Cmd+Tab
- haptic tap on presses, a second one at 400 ms on every control with a hold, none while sliding along a strip

## Setup (macOS)

1. Close the Loupedeck app. Install Companion: `brew install --cask companion`, open `http://127.0.0.1:8000`.
2. Surfaces → Add Surface Integration → Loupedeck. Settings → Protocols: enable shell command support.
3. System Settings → Privacy & Security → Accessibility → add Companion (and enable `zored_stream` when it asks after step 4).
   `zored_stream` also asks for Calendars + Reminders (full access), Bluetooth (headphones) and Automation (Amphetamine, iTerm2, Viscosity, System Events) *(assumed: the full list of prompts on a fresh Mac)*; [macos.md](docs/macos.md#things-that-need-the-user).
4. Run:

```bash
make zored_stream-cert
make zored_stream deploy
make module
make companion
make agent-hooks
```

`make test` checks pages against the daemon and runs the Go tests (CI runs the same).
`make module` clones and builds the [Loupedeck module fork](https://github.com/zored/companion-surface-loupedeck/tree/zored_stream)
next to this repo, points Companion's developer modules path at it, restarts Companion, switches the Loupedeck integration
to the `dev` version and sets the haptics (`HAPTIC`, `HAPTIC_HOLD_MS`, `HAPTIC_HOLD_CONTROLS` from `companion/hold-controls.txt`).
`make companion` starts Companion at login with no window (`start_minimised` + a login item).
`make agent-hooks` adds the Claude Code hooks for [agents waiting for me](docs/agents.md) to `~/.claude/settings.json`; rerunning replaces them.

Yandex Music: `make ymusic-launcher` builds a launcher that keeps its Accessibility tree live while hidden ([ymusic.md](docs/ymusic.md)).

Requirements: Xcode Command Line Tools (`clang`), `m1ddc` (KVM), Go, Node 22+ with `corepack`, Python 3.

## Layout

| Path | What |
|---|---|
| `tools/gen_pages.py` | Page definitions → `companion/*` (mac) and `companion/windows/*` |
| `tools/deploy.mjs`, `tools/trpc.mjs` | Imports pages and creates custom variables in a running Companion over its internal tRPC API |
| `tools/check_contract.py` | CI check: page URLs hit daemon routes, variables match, кнопки gated by choice mode, hooks fail-safe |
| `tools/zored_stream/` | the one daemon ([daemon.md](docs/daemon.md)): calls, volume, Now Playing (perl child), seek, Yandex Music like, brightness, Claude limits + model, agents, choice mode, dictation, scroll, keep-awake, media keys, headphones, wi-fi, KVM, calendar, keyboard layout, emoji, timers |
| `tools/agent_hooks.py` | Adds the Claude Code hooks for agents waiting to `~/.claude/settings.json` |
| `tools/zored_stream/icons/` | Bootstrap Icons (pinned, MIT) for pages and the daemon |
| `tools/surface.mjs` | Switches the Loupedeck integration to a module version and sets surface config over tRPC |
| `tools/companion_config.py` | Edits the Companion launcher's `config.json` |
| `tools/sniff.py` | Raw device event sniffer |
| `docs/` | [Invariants](docs/invariants.md), [Controls](docs/controls.md), [Companion](docs/companion.md), [daemon](docs/daemon.md), [macOS helpers](docs/macos.md), [VK WorkSpace calls](docs/vkcall.md), [Yandex Music](docs/ymusic.md), [AI limits](docs/ai.md), [AI model switch](docs/ai-switch.md), [agents + dictation + choice mode](docs/agents.md), [emoji](docs/emoji.md), [Windows port](docs/windows.md), [device](docs/device.md), [architecture](docs/architecture.html) |

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
