# Yandex Music: like and dislike

macOS only. Verified on Яндекс Музыка 5.30.3 (`ru.yandex.desktop.music`, Electron 32), macOS 26.

## Controls

Only while the Now Playing app is Yandex Music (`np_app`); otherwise the hold does nothing.

| Control | Tap | Hold ≥ 400 ms |
|---|---|---|
| play tile `0/2` | play/pause (on release) | like / remove like |
| left knob 2 `1/0` press | play/pause (on release) | like / remove like |

- The hold vibrates a second time at 400 ms (module patch, any control).
- The left strip shows `💖 ` before the track when liked, `💔 ` when disliked, from `ym_like`.
  Only emoji missing from the text fonts render in colour: `❤️`/`♥` come out white, `🩷` is missing (verified in the web preview).

## How it works

The like lives only in the app UI, so `ymusic` drives the player bar through Accessibility.

- The window is a Chromium web view: setting `AXManualAccessibility` on the app element exposes the DOM.
- Player bar: `AXGroup` with DOM class `PlayerBar_root__…`. Like: `AXCheckBox` (`AXToggleButton`) with class
  `PlayerBarDesktop_likeButton__…`, `AXDescription` «Нравится», `AXValue` 0/1. Dislike: `AXCheckBox` «Не нравится»;
  the bar shows it only in some contexts (not in an album/radio context like «PH Electro Mix Radio»). No control sends
  `/dislike` now: holding the knob for dislike did nothing in such contexts, so it toggles the like too.
- `AXPress` toggles it; works with the app hidden. The UI updates optimistically within ~70 ms, the server call follows.
- Two presses within ~1 s race inside the app: the UI may show the wrong state for seconds. `ymusic` ignores presses
  within 1.5 s of the previous one.
- Finding the bar walks ~1100 AX nodes (~60 ms); the bar element is cached and re-found when it dies.

## ymusic

`tools/ymusic/main.swift`, `make ymusic` → `~/.local/bin/ymusic` + LaunchAgent `com.zored.stream.ymusic`.
Log: `~/Library/Logs/zored-stream/ymusic.log`. `make ymusic-uninstall` removes it.

- Needs **ymusic** itself in System Settings → Privacy & Security → Accessibility (a LaunchAgent inherits nothing).
  Without it the daemon calls `AXIsProcessTrustedWithOptions` with a prompt: macOS adds `ymusic` to the list and shows a dialog;
  enable it there, then `launchctl kickstart -k gui/$(id -u)/com.zored.stream.ymusic`. Adding the binary by hand with `+`
  did not work (verified once). The grant is tied to the cdhash of the ad-hoc signed binary: after `make ymusic` remove the entry
  with `−` and enable it again from the prompt (verified: `tccd` logs `Failed to match existing code requirement` with both cdhashes).
- HTTP on `127.0.0.1:47115`: `POST /like`, `/dislike` → `204`; `409` (reason in the log) when Now Playing is another app,
  the button isn't in the bar, or the previous press was < 1.5 s ago. Reads `np_app` from Companion before pressing.
- Pushes `ym_like` every 0.25 s on change, re-sent every 10 s: `liked`, `disliked`, `none`, `off` (app not running),
  `unknown` (no Accessibility, no player bar or button).
- `np_app` comes from `streamd` (`MRMediaRemoteGetNowPlayingClient` + `MRNowPlayingClientGetBundleIdentifier`).

## Dead ends (verified)

- MediaRemote `LikeTrack` (21): the app registers only commands 0–5 and 24. Now Playing has no track id either
  (`ContentItemIdentifier` is a random UUID).
- Local API: the only listening port belongs to `electron-updater` (MacUpdater proxy, 401). Main↔renderer IPC carries
  only play/pause/next/prev for the tray.
- `yandexmusic://` deeplinks only navigate.
- Hotkeys `F` (like) / `D` (dislike): `keydown` inside the window, need focus, disabled in generative contexts
  (`isGenerativeContext`), and give no state.
- Not tried: CDP via `--remote-debugging-port` (Electron fuses allow it, `EnableNodeCliInspectArguments` is on too) means
  relaunching the app and an open debug port for any local process. The unofficial API
  (`users/{uid}/likes/tracks/add`, `.../{id}/remove`, Ynison for the current track) needs an OAuth token and the app's
  heart may lag behind the server.
