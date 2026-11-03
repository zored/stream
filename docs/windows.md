# Windows port

Plan for running the same controller setup on the Windows PC while the Mac keeps working.
**Verified** = observed on the Mac on 2026-11-03, **assumed** = read or recalled, nothing here was checked on Windows yet.

## Constraints

- The PC has Codex only: no Claude Code, no Claude Desktop. It has its own media players and no VK WorkSpace.
- The Mac setup stays as is: one source (`tools/gen_pages.py`, `tools/zored_stream/`) builds both, mac output unchanged.
- One controller, one owner of its serial port: Companion on whichever machine the cable is in.

## Per control (MEDIA page)

Names as in [companion.md](companion.md): тайл ряд R, колонка C; кнопка N; knobs and strips by side and row.

| Control | macOS | Windows | Mechanism on Windows |
|---|---|---|---|
| left knob 1: volume, press = mute | CoreAudio | keep | Core Audio `IAudioEndpointVolume` on the default render endpoint (`SetMasterVolumeLevelScalar`, `SetMute`), poll as now (assumed) |
| left strip: volume bar, swipe | daemon | keep | same `/volume/drag`, bar code shared |
| left knob 2: prev/next, press = play, hold = like | `:47113/key/*`, Yandex Music AX | keep, hold dropped | `POST :47113/key/{play,next,prev}` → `SendInput` `VK_MEDIA_PLAY_PAUSE` / `VK_MEDIA_NEXT_TRACK` / `VK_MEDIA_PREV_TRACK` (assumed); like: see open questions |
| left knob 3: seek | MediaRemote | keep | GSMTC `TryChangePlaybackPositionAsync`; works only if the player exposes the timeline (browsers: when the site sets a `seekto` handler, assumed) |
| тайл ряд 2, колонка 1: cover + ▶/⏸, hold = like | MediaRemote artwork | keep, hold dropped | GSMTC `TryGetMediaPropertiesAsync().Thumbnail` → PNG/JPEG → same `np_art` data URI |
| strip track text, progress, tint | daemon | keep | same `np_*` variables; `np_app` = `SourceAppUserModelId` |
| тайл ряд 3, колонка 4: clock | Companion | keep | internal variables, no daemon |
| тайл ряд 3, колонка 1 + кнопка 2: mic | VK WorkSpace gRPC | repurpose | system mic mute: `IAudioEndpointVolume::SetMute` on the default capture endpoint, same `:47114/mic/down|up`, same `call_mic` (`on`/`off`); works in any call app (proposal) |
| тайл ряд 3, колонка 2 + кнопка 3: camera | VK WorkSpace gRPC | drop (freed) | no system-wide camera toggle |
| тайл ряд 3, колонка 3 + кнопка 4: hang up | VK WorkSpace gRPC | drop (freed) | no generic hang-up |
| тайл ряд 2, колонка 4 + кнопка 6: agents waiting | Claude Code hooks | repurpose: Codex | `notify` → `agent-turn-complete` = done; needs-input only via Codex hooks (below) |
| кнопка 7: dictation | Claude Desktop AX | drop (freed) | no Claude Desktop |
| right strip: AI limits | Claude OAuth usage API | repurpose: Codex | Codex `rate_limits` from its session logs (below) |
| right knob 2: AI model + effort | Claude Desktop AX | repurpose: Codex (later) | rewrite `model` / `model_reasoning_effort` in `config.toml` for new sessions (idea, assumed) |
| right knob 1: brightness, press+turn = controller | DisplayServices | keep | external monitor: DDC/CI via `dxva2` (`GetPhysicalMonitorsFromHMONITOR`, `Get/SetMonitorBrightness`, slow, coalesce turns); laptop panel: WMI `WmiMonitorBrightnessMethods` (assumed) |
| right knob 3: scroll, press+turn = horizontal, click = Ctrl+Tab mode | CGEvent | keep | `SendInput` `MOUSEEVENTF_WHEEL` / `MOUSEEVENTF_HWHEEL`, 120 per line; Ctrl+Tab via `SendInput` keys with the same release rules (assumed) |
| тайл ряд 1, колонка 4: keep-awake, hold = sleep | Amphetamine, IOKit | drop for now (freed) | `SetThreadExecutionState`, `SetSuspendState` later |
| choice mode on кнопки 1–8 | Claude Desktop / iTerm2 AX | off | no Claude Desktop; buttons are not gated |
| haptics, strip ticks, touch | module fork | keep | OS-independent JS (assumed) |

Freed on Windows: тайл ряд 1, колонка 4, тайл ряд 3, колонки 2–3, кнопки 3, 4, 7, plus the free ones from `PLAN.md`. Candidates: `Win+Alt+K`
(Windows 11 "mute in call", assumed), keystrokes, a Codex "new session" button.

### Codex limits (verified shape, on the Mac)

Codex 0.153.2 writes `token_count` events into `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl` with
`rate_limits.primary` (`window_minutes` 300, `used_percent`, `resets_at`) and `rate_limits.secondary` (`window_minutes` 10080).
That maps 1:1 onto Claude's `session` and `weekly_all`, so `usage.go` keeps its strip and variables and only gets another source:
read the last `rate_limits` from the newest rollout file. No network, no token, no proxy. Stale while Codex is idle;
past `resets_at` the percent counts as 0. On Windows the folder is `%USERPROFILE%\.codex` (assumed).

### Codex waiting

- `notify` in `config.toml` runs a program with the event JSON as the last argument, only `agent-turn-complete` (assumed, docs).
  On the Mac the slot is taken by Computer Use; on the PC it is probably free.
- `codex features list` shows `hooks` as `stable`, off (verified on the Mac). If its events cover approvals, they give
  needs-input; if the JSON matches Claude's, `/agent/event` takes them as is. Check on the PC first.
- Shared `agents.go`; per OS: pid check (`syscall.Kill` → `OpenProcess` + `GetExitCodeProcess`) and focus (Windows Terminal
  or the Codex app window via `SetForegroundWindow`, assumed; no per-tab focus by id).

## One source, two profiles

### Pages (done)

`gen_pages.py <out> [mac|windows]`, default `mac`. `make pages` writes `companion/` (mac, byte-identical to before, verified)
and `companion/windows/` (verified generated). The windows profile:

- media keys: `curl … :47113/key/<name>`, same as mac;
- `exec` goes through `cmd.exe` there (assumed), so URLs with `?` are double-quoted instead of single-quoted;
- no Yandex Music hold, no keep-awake, camera, hang-up, dictation controls; no `call_cam`, `ym_like`, `dictation`, `power_svg` variables;
- no choice-mode gate on the buttons;
- the AI knob (`1/7`) is still there and posts to `:47117`, which the Windows daemon doesn't serve: drop it or give it the Codex switch (open);
- everything else, ports and variables identical.

CI checks both through `git diff --exit-code companion`; `tools/check_contract.py` checks the Windows profile uses no mac-only port
(calls except `/mic`, Yandex Music, keep-awake) and allows the planned `:47113/key/*` routes.

### Daemon

Same binary name, same ports, same endpoints, same variables. Shared files stay shared (`main.go` HTTP + variables, `bar.go`,
`volume.go`, `seek.go`, `skip.go`, `cover.go`, `tint.go`, `scroll.go`, `agents.go`, `usage.go` minus the token source).

| Module | Port | macOS files | Windows files | Windows |
|---|---|---|---|---|
| volume + media keys | 47113 | `audio_darwin.go` | `audio_windows.go`, `keys_windows.go` | on |
| mic | 47114 | `call.go` (VK gRPC) | `mic_windows.go` (capture endpoint mute) | on |
| Yandex Music like | 47115 | `ymusic_darwin.go` | — | off |
| AI limits | 47116 | `usage.go` + `claude_token.go` (keychain via `/usr/bin/security`) | `codex.go` (portable, could feed the Mac too) | on |
| AI model / dictation | 47117 | `ai_darwin.go`, `dictate_darwin.go` | — (Codex model later) | off |
| brightness | 47118 | `display_darwin.go` | `display_windows.go` (DDC/CI) | on |
| Now Playing + seek | 47119 | perl child + dylib | PowerShell child (below) | on |
| agents | 47120 | `agents.go` (focus via `open`/`osascript`, pid), `agents_desktop.go`, `choice*.go` | `agents_windows.go` | on, Codex source |
| scroll | 47121 | `scroll_darwin.go` | `scroll_windows.go` | on |
| keep awake + sleep | 47122 | `power_darwin.go` (Amphetamine, IOKit sleep) | `power_windows.go` (`SetThreadExecutionState`, `SetSuspendState`) | off |

- Module wiring moves out of `main.go` into `modules_darwin.go` / `modules_windows.go`, so the Windows build never links the
  mac-only modules; no stubs.
- Now Playing on Windows: a `powershell.exe` child (Windows PowerShell 5.1 projects WinRT,
  `Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager`), the same protocol as the perl child:
  `track` JSON lines on stdout, `seek <s>` on stdin. Keeps Go free of WinRT bindings; `winrt-go` is plan B (assumed both).
- No cgo on Windows: `golang.org/x/sys/windows` (already a dependency) for `SendInput`, `OpenProcess`, `dxva2`; COM for Core Audio
  by hand or `go-ole` / `go-wca` (assumed).
- Paths: Companion DB `%APPDATA%\companion\v5.1\db.sqlite`; Windows has no `sqlite3` CLI, so the controller brightness comes
  from a pure-Go reader (`modernc.org/sqlite`) or Companion's API.
- Log: `-log %LOCALAPPDATA%\zored-stream\zored_stream.log`, built with `-ldflags=-H=windowsgui` so no console window opens.
- Runs in the user's session (not a service): `SendInput`, GSMTC and DDC need the interactive desktop.

## Install and autostart without `make`

On the PC, by hand (user):

1. Loupedeck / Razer software: quit it and remove it from autostart (Task Manager → Startup apps), it holds the serial port.
2. `winget install Git.Git OpenJS.NodeJS.LTS Python.Python.3.11 GoLang.Go`; Companion 5.1.1 installer from GitHub releases
   (same version as the Mac); JetBrains Mono installed for all users (Companion's SVG text may not see per-user fonts, assumed).
3. Companion: Settings → Protocols, HTTP API on, shell command support on.
4. Clone this repo and the fork `zored/companion-surface-loupedeck` (branch `zored_stream`) side by side.
5. Module: `corepack yarn install`, `corepack yarn build` in the fork; `python tools\companion_config.py enable_developer=true
   dev_modules_path=<folder with a loupedeck link to the fork>` (the script picks `%APPDATA%\companion\config.json` on Windows, assumed path);
   `mklink /J` for the link; restart Companion; `node tools\surface.mjs dev …` with the values from the `Makefile`.
6. Daemon: `go build -ldflags=-H=windowsgui -o %LOCALAPPDATA%\zored-stream\zored_stream.exe .` in `tools\zored_stream`; then a
   logon task with restart on failure:
   ```powershell
   $a = New-ScheduledTaskAction -Execute "$env:LOCALAPPDATA\zored-stream\zored_stream.exe" -Argument "-log `"$env:LOCALAPPDATA\zored-stream\zored_stream.log`""
   $s = New-ScheduledTaskSettingsSet -RestartCount 999 -RestartInterval (New-TimeSpan -Minutes 1) -ExecutionTimeLimit 0
   Register-ScheduledTask zored_stream -Action $a -Trigger (New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME) -Settings $s
   ```
   Rollback: `Disable-ScheduledTask zored_stream` / `Unregister-ScheduledTask zored_stream`.
7. Pages: `node tools\deploy.mjs --vars companion\windows\custom-variables.json companion\windows\media-page.companionconfig:1 companion\test-page.companionconfig:2`.
8. Companion at login, minimised: `companion_config.py start_minimised=true` and a shortcut in `shell:startup` (or the launcher's own option, check).

Once steps 5–8 work by hand, fold them into `tools/windows/setup.ps1` with the `make` target names (`module`, `zored_stream`,
`deploy`, `companion`).

### CI

The macOS job already runs `go vet` + `go test`. A `windows-latest` job: `go vet ./...`, `go test ./...`, `go build -ldflags=-H=windowsgui` in `tools/zored_stream`. It lands
together with the per-OS module split (step 3 below); before it the Windows build fails on the mac-only symbols.

## Steps

Each step is tried on its own and rolled back on its own; the Mac is untouched except where noted.

| # | Where | Step | Checkpoint | Rollback |
|---|---|---|---|---|
| 0 | Mac | profile in `gen_pages.py`, `companion/windows/`, `companion_config.py` path (done) | mac `companion/` unchanged, CI green | revert |
| 1 | PC, user | Companion + upstream Loupedeck module, TEST page | every control logs `TEST …`; check wake from sleep | uninstall Companion, re-enable Loupedeck |
| 2 | PC, user | module fork (install step 5) | haptic ticks on strip slide, hold haptic | `enable_developer=false` |
| 3 | Mac | daemon split: `modules_*.go`, `*_windows.go` for volume + keys + scroll, CI `windows-latest` | `make zored_stream` on the Mac works as before, CI green | revert |
| 4 | PC, user | daemon by hand (`zored_stream.exe` in a terminal) + windows MEDIA page | volume knob/strip, play/next/prev, scroll | stop it, re-import TEST |
| 5 | Mac → PC | Now Playing PowerShell child: `np_*`, cover, seek | track, cover, seek on each player | stop the daemon |
| 6 | Mac → PC | Codex limits on the right strip, Codex waiting | strip moves with Codex use; LED after a turn ends | flags off |
| 7 | Mac → PC | brightness DDC/CI, mic mute | knob moves the monitor; mic LED follows the system mute | flags off |
| 8 | PC, user | logon task, Companion autostart, `setup.ps1` | reboot → everything up without a terminal | disable the task |

## Open questions (user)

- Players on the PC: which apps (Spotify, Yandex Music app, browser)? Decides whether GSMTC covers them and whether like gets a Windows path.
- Codex: CLI in Windows Terminal, the Codex app, or both? Decides focus and the `notify` setup.
- Wake from sleep: wanted? Device Manager → the controller's USB serial device → Power Management →
  "Allow this device to wake the computer"; a CDC serial device may not offer it (assumed).
- Monitor: external over DDC/CI (which model) or a laptop panel?
- Freed controls: system mic mute on the former call mic OK? What goes on camera / hang-up / dictation spots, or keep them dark?
- One controller moved by cable between Mac and PC, or a second one?
