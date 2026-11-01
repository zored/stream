# macOS helpers

Two helpers back the Companion pages. Companion calls them with `exec` actions.

## mediakey — play / next / prev

`tools/mediakey/main.swift`, `make mediakey` → `~/.local/bin/mediakey <play|next|prev|volup|voldown|mute>`.

- Posts system-defined media key events (`NSEvent` subtype 8 → `CGEvent.post(.cghidEventTap)`): reaches whatever owns
  Now Playing, browser tabs included, like a real media key.
- Needs **Companion** in System Settings → Privacy & Security → Accessibility (the child process inherits it).
  Without it the helper exits 1 and Companion logs `Shell command failed … no Accessibility permission`.
- Gotcha: the process must sleep ~100 ms after posting. Exiting right away drops the events silently, even with every permission check passing.
- Dead end: private `MRMediaRemoteSendCommand` returns true but browsers ignore it.

## streamd — volume and Now Playing

`tools/streamd/streamd.m`, an Objective-C dylib run by `/usr/bin/perl tools/streamd/streamd.pl`. `make streamd` builds it,
installs the LaunchAgent `com.zored.stream.streamd` and (re)starts it with `launchctl kickstart -k`
(a `bootout` + `bootstrap` pair races). Log: `~/Library/Logs/zored-stream/streamd.log`. `make streamd-uninstall` removes it.

- HTTP on `127.0.0.1:47113`: `POST /volume/up`, `/volume/down` (acceleration 1 → 4 % per detent by tick interval),
  `/volume/mute`, `/volume/drag?dy=<px>` (0.4 %/px). Companion knobs call it with `curl -fsS -m 1 -X POST …`.
- Volume via CoreAudio (`kAudioHardwareServiceDeviceProperty_VirtualMainVolume`, same scale as the system slider). No native HUD:
  the strip is the HUD.
- Pushes Companion custom variables (must exist: `make deploy` creates them from `companion/custom-variables.json`):
  - `volume` (text, `MUTE` when muted), `volume_anim` (eased 30 fps), `volume_alpha` (0–100, fade in fast, out slow; any change incl. keyboard + 1 s)
  - `np_playing` (`1`/`0`), `np_title`, `np_artist`, `np_track` (`artist — title`), polled every 0.5 s
- Idle CPU ~1.7 %: the 30 fps timer runs only while the volume changes; audio polled every 0.25 s.
- Every 10 s it re-sends all variables, so a Companion restart heals itself.

### Why perl

Since macOS 15.4 MediaRemote returns nothing (`MRMediaRemoteGetNowPlayingInfo` → nil) to third-party processes.
`/usr/bin/perl` is Apple-signed with the right entitlement; loading our dylib into it with `DynaLoader` and calling an XSUB
entry point (`streamd_main`) gets the data. Build the dylib for `arm64` and `arm64e` (perl is arm64e).
Same trick as [ungive/mediaremote-adapter](https://github.com/ungive/mediaremote-adapter).

## Things that need the user

Auto mode blocks these; ask the user to run them:

- `make streamd` (installs a LaunchAgent)
- `make patch-module` (edits the installed Companion module) + restarting Companion
- granting Accessibility to Companion

## Sleep

The device has no HID interface, so it can't wake a sleeping Mac (verified: presses do nothing while asleep, the status LED stays on).
