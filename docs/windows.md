# Windows port

Not started. What carries over and what has to be rebuilt, based on the macOS setup.

## Carries over unchanged

- Companion itself, the Loupedeck surface module, the grid ([companion.md](companion.md)).
- Page files `companion/*.companionconfig` and `companion/custom-variables.json`: layout, expressions, variables, clock, volume strip.
- `tools/deploy.mjs` (Node 22+, built-in `WebSocket`) and `tools/gen_pages.py`.
- The module fork and `tools/surface.mjs`; `tools/companion_config.py` and `make companion-restart` need the Windows paths (`%APPDATA%`) and a Windows restart.

## Needs a Windows build

`exec` commands are the only OS-specific part. Keep the CLIs identical so pages only swap the command prefix:

| macOS | Windows replacement |
|---|---|
| `mediakey play\|next\|prev` (system media key events) | `SendInput` with `VK_MEDIA_PLAY_PAUSE` / `VK_MEDIA_NEXT_TRACK` / `VK_MEDIA_PREV_TRACK`; no permission needed |
| `zored_stream` volume (CoreAudio) | Core Audio `IAudioEndpointVolume` (`SetMasterVolumeLevelScalar`, `SetMute`) + `IAudioEndpointVolumeCallback` for external changes |
| `zored_stream` Now Playing (MediaRemote via perl) | `GlobalSystemMediaTransportControlsSessionManager` (WinRT): playback status, title, artist; also has play/next/prev |
| LaunchAgent | Task Scheduler "at logon" task or a startup shortcut |
| `curl` in knob actions | `curl.exe` ships with Windows 10+ |

Decided: `zored_stream` in Go with `*_windows.go` files ([daemon.md](daemon.md)) covers volume, Now Playing and media keys, with the same HTTP endpoints on `127.0.0.1:47113`
and the same Companion variables. Then `gen_pages.py` takes the OS as a parameter and emits the matching command.

## Open questions

- `make` → npm scripts or plain `node`/`python` calls.
- Whether the device wakes a sleeping Windows PC (Device Manager → Power Management → "Allow this device to wake the computer").
