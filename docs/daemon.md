# One daemon: `zored_stream`

Decision record for merging `streamd`, `vkcall`, `ymusic` (and later `mediakey`) into one binary. Done 2026-11-02:
the old helpers are gone, `mediakey` followed on 2026-11-03 (`:47113/key/*`). Current modules: [zored_stream today](#zored_stream-today).
**Verified** = observed on this Mac on 2026-11-02, **assumed** = read or recalled, not checked.

## Decision

Go. Grow the spike `tools/zored_stream/` into the daemon: copy `vkcall` in as-is, call macOS C APIs through cgo,
keep MediaRemote in a `/usr/bin/perl` child. Rust is the runner-up and only wins on the Windows port.

## Before the merge (2026-11-02)

| Helper | Language | Runs as | HTTP | Companion variables | Permissions |
|---|---|---|---|---|---|
| `streamd` | Objective-C dylib in `/usr/bin/perl` | LaunchAgent | `:47113` `POST /volume/{up,down,mute}`, `/volume/drag?dy=` | `volume`, `volume_anim`, `volume_alpha`, `np_playing`, `np_title`, `np_artist`, `np_track`, `np_app` | none; perl's Apple signature unlocks MediaRemote |
| `vkcall` | Go (grpc-go, raw protowire) | LaunchAgent | `:47114` `POST /mic/{down,up}`, `/cam/down`, `/call/end`, `409` without a call | `call_mic`, `call_cam` | none |
| `ymusic` | Swift (AX) | LaunchAgent | `:47115` `POST /like`, `/dislike`, `409` on refusal | `ym_like`; reads `np_app` back from Companion | Accessibility for `ymusic` itself, tied to the ad-hoc cdhash |
| `mediakey` | Swift CLI | Companion `exec` per press | none | none | Accessibility inherited from Companion |

Common shape (verified, read in the sources): a loopback HTTP server, `POST /api/custom-variable/<name>/value` on change,
everything re-sent every 10 s, `KeepAlive` LaunchAgent with a log in `~/Library/Logs/zored-stream/`. Three copies of the same skeleton
in three languages, three LaunchAgents, three logs, and `ymusic` asks Companion for `np_app` that `streamd` computed.

Found on the way (verified):

- `ymusic` crashed: 30 reports in `~/Library/Logs/DiagnosticReports` on 2026-11-02, `launchd` showed 112 runs.
  "Thread stack size exceeded due to excessive recursion" in `find` → `hasClass` → `AXUIElementCopyAttributeValue`.
  `KeepAlive` hid it. Fixed with a bounded breadth-first walk ([ymusic.md](ymusic.md)).
- This Claude session inherited a stale `CGO_LDFLAGS` (`-lvips` from a removed Homebrew Cellar). Not set in any dotfile;
  it comes from the environment the app started with. cgo builds fail until the app restarts, or with `CGO_LDFLAGS=`.

## Options

| | Go | Rust | Swift | Keep as is |
|---|---|---|---|---|
| gRPC to `vkws-call-service` | reuse `vkcall` as-is (verified working) | `tonic` with a raw codec or `prost` structs, rewrite | `grpc-swift` 2 needs SwiftPM + NIO, rewrite | works |
| CoreAudio, AX, CGEvent, IOKit power | plain C APIs, direct cgo (CoreAudio verified in the spike) | `objc2-*` generated crates, `unsafe` (assumed) | native | native per helper |
| NSWorkspace, IOBluetooth, EventKit | ObjC files in the package (cgo compiles `.m`, assumed) | `objc2-app-kit`, `objc2-io-bluetooth`, `objc2-event-kit` (assumed) | native | new helper per feature |
| MediaRemote | perl child streaming JSON lines; the dylib stays ObjC | same; Rust has no stable `arm64e` target, perl is `arm64e` (assumed) | same | works |
| One Accessibility grant | yes, if signed with a stable identity | same | same | one grant per helper, re-granted on every rebuild |
| Windows port | `GOOS=windows`, no cgo: `x/sys/windows`, `go-ole` for Core Audio; WinRT media session through a thin wrapper (assumed, the weak spot) | `windows-rs`, first-class WinRT + COM (assumed, the strong spot) | no | C# `streamd.exe` as in [windows.md](windows.md) |
| Toolchain / CI | installed, CI already sets up Go on `macos-latest` (verified) | not installed here (`cargo` missing, verified); add `rustup` to CI | installed, no deps needed for AX | as now |
| Migration effort | `vkcall` 560 lines copied, `streamd.m` C parts copied, `ymusic` ~150 lines of AX to C | rewrite ~1100 lines, learn objc2 | rewrite `vkcall` gRPC | none |
| Upcoming features (AI limits, Claude hooks, calendar, keep-awake, headphones, Dock badges, keystrokes) | HTTP/JSON/keychain trivial in Go; macOS bits are C or small ObjC | same, more ceremony | best for macOS bits, worst for HTTP + gRPC | one more helper each |

Why Go and not Rust: the hard, already-debugged part is `vkcall` and it is Go; the macOS APIs the backlog needs
(CoreAudio, AX, CGEvent, `IOPMAssertionCreateWithName`, Dock badges via AX) are C, so cgo calls them without bindings.
Rust's real edge is `windows-rs`, but on Windows most features differ anyway (no VK WorkSpace gRPC port check, no AX),
and the shared part (HTTP, Companion variables, volume acceleration, AI limits, hooks) is plain Go that cross-compiles.

Why one binary at all: one Accessibility grant (needed by `ymusic`, keystrokes, Dock badges, `mediakey` if moved), one
Companion client, `np_app` shared in memory, one log, one LaunchAgent. The grant only sticks across rebuilds with a stable
signature: TCC keeps the designated requirement, which for ad-hoc signing is the cdhash (verified, see [ymusic.md](ymusic.md));
signing with a certificate (Apple Development or a self-signed code-signing cert in the login keychain) makes it identifier + cert
(assumed, standard TCC behaviour). That holds for any language.

## zored_stream today

`tools/zored_stream/`, one Go binary, one HTTP server per module on the module's old port, so pages need no change:

| Module | File | Address (flag) | Status |
|---|---|---|---|
| VK WorkSpace calls | `call.go`, from `vkcall` (removed) | `127.0.0.1:47114` (`-call`) | verified in a call: mic tap/hold, camera, hang-up; `409` without a call |
| Volume | `volume.go`, `bar.go`, `audio_darwin.go` (CoreAudio via cgo) | `127.0.0.1:47113` (`-volume`) | verified: up/down/drag/mute move the system volume, `bar_alpha` fades to 0, external changes arrive via CoreAudio listeners (no poll); the strip on the device looks as before (user) |
| Now Playing | `nowplaying.go`, `nowplaying/` (ObjC dylib in `/usr/bin/perl`, JSON lines on stdout) | — (`-nowplaying-script`, `-nowplaying-lib`) | verified: `np_*` from Yandex Music, the perl child exits with the daemon |
| Yandex Music like | `ymusic_darwin.go`, `ax_darwin.m` (AX in C, BFS with a 20 000-node cap) | `127.0.0.1:47115` (`-ymusic`) | verified: `ym_like` read, like/unlike pressed, `409` on a press within 1.5 s, grant kept after a rebuild |
| Brightness | `brightness.go`, `display_darwin.go` (DisplayServices via cgo), [macos.md](macos.md) | `127.0.0.1:47118` (`-brightness`) | verified: turns move the Mac display, controller value goes to `ctl_brightness`; device brightness: user |
| Claude usage | `usage.go`, [ai.md](ai.md) | `127.0.0.1:47116` (`-usage`) | verified: fetch through the proxy, tile, `GET /usage` |
| Claude Desktop model + effort, dictation | `ai_darwin.go`, `dictate_darwin.go`, `ax_darwin.m`, [ai-switch.md](ai-switch.md), [agents.md](agents.md#dictation) | `127.0.0.1:47117` (`-ai`) | verified: knob turns switch the new-chat pickers, read-back every second; dictation gestures; frontmost pid from `NSWorkspace` notification |
| Seek | `seek.go`, [macos.md](macos.md) | `127.0.0.1:47119` (`-seek`) | verified with Yandex Music |
| Agents waiting, choice mode | `agents.go`, `agents_desktop.go`, `choice.go`, `choice_darwin.go`, [agents.md](agents.md) | `127.0.0.1:47120` (`-agents`) | verified on the device |
| Scroll, Ctrl+Tab | `scroll.go`, `scroll_darwin.go`, [macos.md](macos.md) | `127.0.0.1:47121` (`-scroll`) | verified on the device |
| Keep-awake, sleep | `power.go`, `power_darwin.go`, [macos.md](macos.md) | `127.0.0.1:47122` (`-power`) | keep-awake verified; sleep not triggered; Amphetamine read on IOPM assertion change |
| Headphones | `headphones.go`, `headphones_darwin.{go,m,h}`, [macos.md](macos.md) | `127.0.0.1:47123` (`-headphones`) | see macos.md |
| Media keys, tile swipes | `mediakey*.go`, `tileswipe.go`, `skip.go`, [macos.md](macos.md), [device.md](device.md#touch) | `127.0.0.1:47113` (`/key/*`, `/tile/swipe`) | see macos.md |
| Wi-fi + Viscosity | `wifi.go`, `wifi_darwin.go` (CoreWLAN, Control Center via `osascript`), [macos.md](macos.md) | `127.0.0.1:47124` (`-wifi`) | see macos.md |
| KVM | `kvm.go` (Wake-on-LAN, `m1ddc`), [macos.md](macos.md) | `127.0.0.1:47125` (`-kvm`) | see macos.md |
| Calendar + reminders, tile layers | `calendar.go`, `calendar_darwin.go` (EventKit), `layers.go` (`tile_24`), [macos.md](macos.md) | `127.0.0.1:47126` (`-calendar`, `/tile/24/press`) | see macos.md |
| Keyboard layout | `input.go`, `input_darwin.go`, [macos.md](macos.md) | `127.0.0.1:47127` (`-input`) | see macos.md |
| Emoji picker | `emoji.go`, `emoji_darwin.go`, `emoji_data.go`, [emoji.md](emoji.md) | `127.0.0.1:47128` (`-emoji`) | see emoji.md |
| Timers + alarms | `timers.go`, [macos.md](macos.md) | `127.0.0.1:47129` (`-timers`) | see macos.md |
| Window switcher | `switcher.go`, `switcher_darwin.go`, `switcher_darwin.m`, `picker.go` (input routing shared with emoji), [windows-switch.md](windows-switch.md) | `127.0.0.1:47132` (`-switch`) | see windows-switch.md |
| Weather | `weather.go`, `layers.go` (`tile_23`), [weather.md](weather.md) | `127.0.0.1:47131` (`-weather`) | see weather.md |
| Orientation (read-only) | `orientation.go`, `stripflip.go`; the fork owns it ([companion.md](companion.md#loupedeck-surface-module-fork), A12) | — (reads `~/.config/zored-stream/orientation`, publishes `strip_flip` with the bars, turns emoji glyphs) | verified on the device (user) |

- `make zored_stream-cert` runs `tools/zored_stream/cert.sh`: a self-signed `zored-stream` code-signing cert in the login keychain, trusted for code signing
  (asks for the password); rerunning it only re-applies the trust. Verified: the identity imports into the login keychain
  and shows as `CSSMERR_TP_NOT_TRUSTED` until trusted; `codesign` refuses an untrusted one; `-r trustAsRoot` is rejected for a
  self-signed cert (`SecTrustSettingsSetTrustSettings: One or more parameters … not valid`), `trustRoot` works. A signed build's
  designated requirement is `identifier "com.zored.stream.zored_stream" and certificate leaf = H"…"`, no cdhash (verified).
- `make zored_stream-bin` builds and signs with `SIGN` (default `zored-stream`, `make build`/CI use `SIGN=-`), identifier
  `com.zored.stream.zored_stream`. `make zored_stream` also installs the LaunchAgent, log `~/Library/Logs/zored-stream/zored_stream.log`.
- `CGO_LDFLAGS=` in front of `make` while the stale environment is around.

## Migration

Each step keeps the old helper until the new path is verified on the device. Modules keep their old ports, so a switch is
"uninstall the helper, restart `zored_stream`", no `gen_pages.py` change.

1. Signing: done, `make zored_stream-cert`.
2. Skeleton: done (`make zored_stream`, LaunchAgent, CI).
3. `vkcall` → `call.go`: done, verified in a call (mic tap/hold, camera, hang-up), `tools/vkcall/` removed. Fixed on the way:
   the first status snapshot without a call was skipped, so the state stayed `unknown` and the 10 s resend blinked the LEDs yellow.
4. `streamd` volume: done (animation, drag, mute, acceleration), on `:47113`.
5. Now Playing: done, perl child with `libnowplaying.dylib`. `streamd` uninstalled, `tools/streamd/` removed.
6. `ymusic`: done, AX walk in C (`ax_darwin.m`), `np_app` from memory, on `:47115`. The Accessibility grant survived a rebuild
   with a new cdhash (verified). `tools/ymusic/` removed.
7. Done 2026-11-03: `mediakey` as `POST :47113/key/<name>`; the CLI is deleted.
8. New features land as packages: keep-awake (`IOPMAssertion`), headphones (IOBluetooth ObjC + CoreAudio), AI limits, Claude hooks
   (`POST /agent/...`), Dock badges (AX), calendar (EventKit ObjC).
9. Windows: `*_windows.go` files behind the same endpoints and variables, replaces the C# plan in [windows.md](windows.md) if the
   WinRT media session wrapper works; otherwise only that part goes C#.

## Decided (2026-11-02)

- Go, Windows included: the Windows side is written in Go too.
- Signing: a self-signed code-signing certificate in the login keychain.
- ~~The user runs `make zored_stream`~~ superseded the same day: sessions run `make` targets themselves; permission grants stay with the user.
