# One daemon: `zored_stream`

Decision record for merging `streamd`, `vkcall`, `ymusic` (and later `mediakey`) into one binary.
**Verified** = observed on this Mac on 2026-11-02, **assumed** = read or recalled, not checked.

## Decision

Go. Grow the spike `tools/zored_stream/` into the daemon: copy `vkcall` in as-is, call macOS C APIs through cgo,
keep MediaRemote in a `/usr/bin/perl` child. Rust is the runner-up and only wins on the Windows port.

## Today

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
| Volume | `volume.go`, `audio_darwin.go` (CoreAudio via cgo) | `127.0.0.1:47120` (`-volume`), moves to `:47113` when `streamd` goes | verified: up/down move the system volume and `volume`; no animation, drag, Now Playing |

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

1. Signing (user): `make zored_stream-cert`; after the first AX module, check that a rebuild keeps the Accessibility grant.
2. Skeleton: done (`make zored_stream`, LaunchAgent, CI).
3. `vkcall` → `call.go`: done, verified in a call (mic tap/hold, camera, hang-up), `tools/vkcall/` removed. Fixed on the way:
   the first status snapshot without a call was skipped, so the state stayed `unknown` and the 10 s resend blinked the LEDs yellow.
4. `streamd` volume: animation (`volume_anim`, `volume_alpha`, 30 fps ticker only while changing), drag; switch knobs, keep `streamd` for Now Playing.
5. Now Playing: `zored_stream` starts `/usr/bin/perl streamd.pl libnowplaying.dylib`, the dylib prints JSON lines on change, the daemon
   restarts it on exit and pushes `np_*`. Uninstall `streamd`.
6. `ymusic` → AX in C via cgo, with an iterative, depth-limited walk (fixes the crash); `np_app` from memory. Uninstall `ymusic`.
7. Optional: `mediakey` as `POST /key/<name>`; drops the `exec` and Companion's Accessibility.
8. New features land as packages: keep-awake (`IOPMAssertion`), headphones (IOBluetooth ObjC + CoreAudio), AI limits, Claude hooks
   (`POST /agent/...`), Dock badges (AX), calendar (EventKit ObjC).
9. Windows: `*_windows.go` files behind the same endpoints and variables, replaces the C# plan in [windows.md](windows.md) if the
   WinRT media session wrapper works; otherwise only that part goes C#.

## Decided (2026-11-02)

- Go, Windows included: the Windows side is written in Go too.
- Signing: a self-signed code-signing certificate in the login keychain.
- The user runs `make zored_stream`, the LaunchAgent install and every helper uninstall (auto mode blocks them); sessions ask when a step needs it.
