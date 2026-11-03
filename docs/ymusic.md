# Yandex Music: like and dislike

macOS only. Verified on Яндекс Музыка 5.30.3 (`ru.yandex.desktop.music`, Electron 32), macOS 26.

## Controls

Only while the Now Playing app is Yandex Music (`np_app`); otherwise the hold does nothing.

| Control | Tap | Hold ≥ 400 ms |
|---|---|---|
| play tile `1/2` | play/pause (on release) | like / remove like |
| left knob 2 `1/0` press | play/pause (on release) | like / remove like |

Play tile swipes (module fork → `:47113/tile/swipe`): up = like, down = dislike, same `pressAndPoll` as the hold; dislike only where the player bar shows it.

- The knob hold vibrates a second time at 400 ms (module fork, `HAPTIC_HOLD_CONTROLS` from `companion/hold-controls.txt`, generated from every control with a hold).
- The left strip shows a heart before the track from `ym_like`: pink when liked, gray broken when disliked. It is an SVG path in `np_svg`
  (Skia has no colour emoji fallback; emoji in the text came out white or missing, verified in the web preview).

## How it works

The like lives only in the app UI, so `zored_stream` drives the player bar through Accessibility.

- The window is a Chromium web view: setting `AXManualAccessibility` on the app element exposes the DOM.
- Player bar: `AXGroup` with DOM class `PlayerBar_root__…`. Like: `AXCheckBox` (`AXToggleButton`) with class
  `PlayerBarDesktop_likeButton__…`, `AXDescription` «Нравится», `AXValue` 0/1. Dislike: `AXCheckBox` «Не нравится»;
  the bar shows it only in some contexts (not in an album/radio context like «PH Electro Mix Radio»). No control sends
  `/dislike` now: holding the knob for dislike did nothing in such contexts, so it toggles the like too.
- Track title: `AXLink` `Meta_albumLink__…` with `AXDescription` «Трек <title> ». The like is published together with it;
  the strip shows the heart only when that title matches `np_title` (prefix match either way).
- The AX tree freezes while the window is occluded or hidden: on 2026-11-02 the bar still said «Still Waiting», like = 1,
  two tracks later (verified). That is the «AX lies» bug: a stale like moved onto new tracks. Toggling
  `AXManualAccessibility` off and on in that state drops the web content from the tree (136 nodes instead of ~1100) until
  the window is shown again (verified); `AXEnhancedUserInterface` is not supported (-25208).
- Fix: launch the app with `--disable-backgrounding-occluded-windows --disable-renderer-backgrounding
  --disable-background-timer-throttling`. With the window hidden (Cmd+H) the AX title then follows every track and a like
  shows up and goes away with its track (verified 2026-11-02). `make ymusic-launcher` builds
  `~/Applications/Яндекс Музыка (stream).app` that does this; put it in the Dock instead of the app. `open --args` is ignored
  when the app is already running: quit it first.
- `AXPress` toggles it; works with the app hidden. The UI updates optimistically within ~70 ms, the server call follows.
- Two presses within ~1 s race inside the app: the UI may show the wrong state for seconds. The daemon ignores presses
  within 1.5 s of the previous one. Seen again on 2026-11-02: like, then unlike 2 s later → still `liked`; a third press 5 s later → `none`.
- Finding the bar walks ~1100 AX nodes (~60 ms); the bar element is cached and re-found when it dies.
  The walk is breadth-first with a visited set and a 20 000-node cap: the recursive version crashed with a stack overflow
  ~11 s after start, 30 times on 2026-11-02 (verified in crash reports); a cycle or a runaway subtree in the AX tree is assumed.

## zored_stream module

`tools/zored_stream/ymusic_darwin.go` + `ax_darwin.m` (AX walk in C) in the [one daemon](daemon.md), was the Swift `ymusic`.
Log: `~/Library/Logs/zored-stream/zored_stream.log`, lines prefixed `ymusic:`.

- Needs **zored_stream** in System Settings → Privacy & Security → Accessibility (a LaunchAgent inherits nothing).
  Without it the daemon calls `AXIsProcessTrustedWithOptions` with a prompt: macOS adds it to the list and shows a dialog;
  enable it there, then `launchctl kickstart -k gui/$(id -u)/com.zored.stream.zored_stream` (the trust answer is cached per process, verified).
- The grant survives rebuilds: the binary is signed with the `zored-stream` certificate, so TCC matches identifier + certificate
  (verified 2026-11-02: cdhash changed, `ym_like` stayed readable). The old ad-hoc `ymusic` lost it on every rebuild
  (`tccd`: `Failed to match existing code requirement`).
- HTTP on `127.0.0.1:47115`: `POST /like`, `/dislike` → `204`; `409` (reason in the log) when Now Playing is another app,
  the button isn't in the bar, or the previous press was < 1.5 s ago. Checks `np_app` in memory before pressing.
- Pushes `ym_like` every 0.25 s on change, re-sent every 10 s: `liked`, `disliked`, `none`, `off` (app not running),
  `unknown` (no Accessibility, no player bar or button).
- `np_app` comes from the Now Playing module (`MRMediaRemoteGetNowPlayingClient` + `MRNowPlayingClientGetBundleIdentifier`).

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
