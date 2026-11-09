# Screen panel

An on-screen controller panel for macOS, shown only while the USB controller is unplugged.
**Verified** = observed here, **assumed** = read or reasoned, not checked.

## What it is

`tools/screen/`: a small Swift (AppKit) app, LaunchAgent `com.zored.stream.screen`. It is a second Companion surface:
Companion renders every frame, the app only shows them and sends input back. No page or daemon logic is duplicated.

```
Companion ──Satellite TCP :16622── screen app ── NSPanel at the top of the screen
   ▲  KEY-PRESS / KEY-ROTATE            │
   └────────────────────────────────────┘  strip drags → zored_stream (:47113 /volume/drag, :47117 /ai/slide, :47128 /emoji/*)
```

## Layout

- One row at the top center of the primary screen (`NSScreen.screens.first`), above all windows, on all Spaces, never takes focus (`NSPanel`, `.nonactivatingPanel`, level `.statusBar`).
- On a display with a camera cutout, `safeAreaInsets` and the two `auxiliaryTop*Area` rectangles define a central exclusion. The left strip + tiles 1–6 sit to its left, tiles 7–12 + the right strip to its right, with 8 pt clearance. Each wing grows outward during hover; magnification is limited to the available width, keeping 8 pt at the display edges. On an unusually narrow display, base sizes and gaps shrink together. A display without a cutout keeps the original continuous row.
- 12 tiles ordered by device column: 4 groups of 3. Group *c* holds тайл ряд 1–3, колонка *c*. Tile number *n* = 1…12 in that order.
- Strips lie horizontally at both ends of the row, bottom edge towards the tiles: the left strip turned 90° counterclockwise
  (device top → screen left), the right strip 90° clockwise (device top → screen right).
- No кнопки 1–8 on screen.
- Small mode: tile 40 pt, strip 40 × 215 pt (48 × 258 turned), continuous corners (4 pt at rest, 8 pt enlarged), clipped artwork and a hairline outline.
- Hover: Dock-style magnification, each element by its own distance to the cursor (cosine falloff, range 200 pt),
  up to 80 pt per tile (= the device tile); grows downwards from a 4 pt top margin. Magnification runs only while the cursor is inside the panel and freezes while any mouse button is held. Hover uses a fast critically damped spring (omega 70) that keeps its velocity when the cursor changes direction; focus/session loss and application changes return to rest, and mouse movement outside the panel clears missed exit events. The panel moves to the primary display when the display configuration changes.
- Companion supplies the pictures. On macOS 26+, a clear native `NSGlassEffectView` sits beneath each picture, batched in `NSGlassEffectContainerView`. macOS 14–25 uses `NSVisualEffectView` blur; it does not provide Liquid Glass refraction. RGB backgrounds are converted to premultiplied RGBA: black up to channel value 8 is transparent, values 8–24 feather into opaque pixels, and brighter content stays opaque. The music cover rectangle (grid `1/2`, 6 % inset) stays opaque, including its black pixels. Glass has no dark tint. Pictures fill the tile without an extra glass edge; the artwork and tile share the same corner radius. The native effect controls refraction and highlights; it is not a custom iOS shader.

## Input

| Mouse | Control |
|---|---|
| click on a tile | tap (down on mouse down, up on mouse up: holds work) |
| drag on a tile ≥ 40 pt | swipe: `POST :47113/tile/swipe?id=<control>&dir=l\|r\|u\|d`, as the fork does |
| wheel over tiles 1–3 | крутилка слева, ряд 1–3 turn |
| wheel over tiles 10–12 | крутилка справа, ряд 1–3 turn |
| right click on tiles 1–3 / 10–12 | that knob's press (down/up) |
| middle click on tile *n* ≤ 8 | кнопка *n* (down/up) |
| drag / wheel along the left strip | `POST :47113/volume/drag?dy=` (device px) |
| drag along the right strip | `POST :47117/ai/slide?y=` (device px, absolute) |
| strip press / release | `POST :47128/emoji/start\|end?s=l\|r&y=` |
| double click on a strip | hide the panel (strip taps do nothing on the device, [controls.md](controls.md)) |
| cursor at the top screen edge (≤ 2 pt) for 3 s while hidden | peek, then show the panel again |

A release goes to the control pressed on mouse down, even outside the panel. A tile swipe waits for the daemon reply:
any status except `404` releases the tile immediately; `404` keeps the normal release on mouse up. A mouse up during the request
waits for that reply, then releases once. Wheel direction follows physical movement (up = knob right), independent of natural scrolling;
mouse wheels use one delta unit per step and precise trackpads accumulate 10 pt per step.
The second click of a strip double click hides the panel without sending another emoji start/end pair.

Strip mouse position → device y (0 … 270, 0 = top): left strip `y = 270 · x / width`, right strip `y = 270 · (1 − x / width)`.

Left-strip drag posts `dy=previousY−currentY`, skipping the first sample and unchanged coordinates; its wheel posts ±5 device px
per step (the daemon uses 0.4 % per px). Right-strip drag posts the changed absolute `y`. Strip cells receive no Satellite presses.
All daemon requests use an ephemeral URLSession with a one-second timeout and log failures.

## Frames

Satellite `ADD-DEVICE` with `LAYOUT_MANIFEST` (verified in Companion 5.1.1: per-control `bitmap {w, h}` style presets,
`KEY-ROTATE` by `CONTROLID`): the 4×8 grid of the Loupedeck page, presets `tile` 160×160 and `strip` 160×287
(2× the largest on-screen size). The surface joins the Loupedeck's surface group so it shows the same page (`node tools/surface.mjs group`). Verified through tRPC: both surfaces belong to `group:zored-stream`, assigned with `surfaceSetGroup` and the physical surface's configuration copied to the group. Companion UI confirmation is pending.
Companion pushes a frame only when a picture changes, through the same debounce as the device: ≤ 60 fps (assumed: same pipeline).
Each displayed element is one `CALayer`. A strip stacks three RGB cells top to bottom, then turns the result left/right.
The fork already removes its 6 px padding: device cells are 48×86, so no extra cropping is applied. The 160×287 preset preserves
that aspect ratio at twice the largest on-screen height. Missing cells are black until their first frame arrives.

## Satellite protocol

Verified by inspection of Companion 5.1.1 `Contents/Resources/main.js` in the preceding implementation session (2026-11-06):

- Server greeting: `BEGIN CompanionVersion=… ApiVersion="1.14.0"`, followed by `CAPS`.
- Client: `ADD-DEVICE DEVICEID=zored-screen PRODUCT_NAME="zored screen" BITMAP_FORMAT=rgb BRIGHTNESS=false LAYOUT_MANIFEST=…`.
- Input: `KEY-PRESS … CONTROLID=… PRESSED=true|false` and `KEY-ROTATE … CONTROLID=… DIRECTION=1|-1`.
  Pressed is false for `false`, `0` or an empty value; rotation uses the sign of the number.
- Server fields are quoted, booleans are `1`/`0`; replies are `CMD OK …` or `CMD ERROR … MESSAGE="…"`.
- `KEY-STATE DEVICEID CONTROLID LOCATION="page/row/col" PRESSED TYPE BITMAP=…` carries base64 raw RGB (`w·h·3` bytes)
  only for bitmap presets. Accepted formats are `rgb` (default), `png`, `webp`.
- Invalid manifests produce `ERROR … MESSAGE="Invalid LAYOUT_MANIFEST"`.

Verified in the installed Companion bundle during this continuation: `e.setTimeout(5e3,()=>e.destroy())` closes a Satellite
socket after five seconds without inbound traffic. The initial probe received all 18 bitmap control ids while the physical controller
was plugged in, then exited with `Satellite closed` (status 1) at the inactivity timeout. After adding the heartbeat, the repeated probe received all 18 ids and exited cleanly (status 0). Companion accepts both surfaces together.

The client answers `PING` with `PONG` and sends a `PING` every two seconds while connected to prevent this timeout.
This connection-only heartbeat is the exception to the no-timers rule; disconnect cancels it. A Satellite error, malformed frame size, closed connection or failed/waiting TCP state logs
and exits with status 1; launchd restarts it. An intentional disconnect sends `REMOVE-DEVICE` before closing.

## When it shows

- USB controller plugged (IOKit matched/terminated, VID `0x1532`, PID `0x0D06`) → the row slides up, Satellite sends `REMOVE-DEVICE` and disconnects. Unplugged → Satellite connects and the row slides down. Verified 2026-11-09: launch with the controller plugged logs `device plugged=true`, keeps no `:16622` socket.
- Double click on a strip slides the row up over 120 ms and hides it. Any USB change clears this user hide: plugging hides anyway, unplugging always shows.
- Hidden by the user, a global `mouseMoved` monitor runs (assumed: needs no Accessibility). Cursor ≤ 2 pt from the top of the primary display starts a peek: over 3 s the row slides down until 10 pt of each resting tile is visible. After 3 s the panel shows completely. Cursor more than 14 pt below the top during the peek → the row flies back up in 80 ms; the next touch of the edge starts a new 3 s peek. The peek ignores mouse input.
- Input is disabled during every transition.
- Idle CPU target ≈ 0: USB notifications, frames and mouse events drive the panel (P2). There is no cursor polling: the 3 s reveal is a one-shot `DispatchWorkItem`, so a cursor resting at the edge needs no events. A primary-screen CADisplayLink drives hover/visibility transitions at the refresh rate reported by AppKit and is invalidated when settled; the 3 s peek runs it at 30 Hz (10 pt in 3 s moves under 0.2 pt per frame). A two-second Satellite heartbeat runs only while connected because Companion closes idle sockets after five seconds; disconnect cancels it. The five-second probe exit is diagnostic only.
- Measured 2026-11-09: with the controller plugged the installed app used 0.01 s CPU in 30 s (`ps`). The AppKit regression spends 0.11–0.21 s CPU during one 3 s peek.

## Install and verify

`make screen` builds and signs `~/.local/bin/screen` with `zored-stream`, then installs `com.zored.stream.screen`.
`make screen-uninstall` unloads the agent. The agent uses `KeepAlive`, a 5 s restart throttle and
`~/Library/Logs/zored-stream/screen.log`. `make build` also builds the app with ad-hoc signing for CI.
`cd tools/screen && swift run screen-checks` runs the pure Swift checks; `make test` includes them.
`swift test` does not work with the Command Line Tools setup used here.

Verified in this continuation: a standalone IOKit run detected the plugged controller (`device plugged=true`);
the LaunchAgent plist passes `plutil -lint`. The plugged-device probe received all 12 tile and 6 strip ids and exited with status 0 after the heartbeat fix; tRPC confirmed the common group. The user confirmed that the panel appears to work visually. Detailed input checks below are pending:

- Confirm the common group in Companion → Surfaces.
- With the controller unplugged, tile 4 press must log `Button 1/0/3 pressed`; wheel on tile 1 changes volume;
  right click on tile 12 toggles scroll mode; middle click on tile 7 presses кнопка 7; left-strip drag changes volume.
- Release outside the pressed tile: confirm no control stays pressed.
- USB transitions (ask the user to handle the cable): plug → the row slides up and the log shows `device plugged=true`; unplug → it slides down, frames arrive. A user hide does not survive either change.
- Double click a strip with the emoji picker closed: hide without changing state. Hold the cursor at the top edge: a 10 pt sliver slides in over 3 s, then the full row. Move away before 3 s: the sliver flies back up.
  If macOS asks for Accessibility, stop for the user to grant permission.
- Change the primary display or disconnect it: the panel follows the primary display.
- Restart Companion: the app exits, launchd restarts it, then reconnects.
- CPU sampling of the installed app (`top`, seven samples at 10 s) observed 0.0–3.9 % with live Companion frames. Cursor inactivity was not controlled, so the idle target remains unverified.

## Open

- macOS reveals the auto-hidden menu bar on the same top-edge move; both show, check how it looks.
- The main display is an OLED (MPG321UX): static white pixels all day. A 1–2 px drift every few minutes if the user wants it.
- iOS later: a PWA speaking Satellite over WebSocket `:16623`, or SwiftUI sharing `ScreenCore`; a paid Apple Developer account
  past the 7-day free signing. Satellite listens on `*` (verified), so the LAN reaches it.

Shutdown regression: `python3 tools/screen/check_shutdown.py` builds a temporary app against an isolated TCP server, delays REMOVE-DEVICE and checks it arrives before EOF with exit 0. Verified RED→GREEN. Termination first cancels the AppKit request, drains disconnect asynchronously, then requests termination again; `terminateLater` stalled this command-line AppKit runloop in the regression.

Panel regression: `python3 tools/screen/check_panel.py` compiles the real AppKit panel with a fixed cursor fixture and posts an in-process focus-loss notification. It checks hover interpolation, collapse on focus loss, animated hide/reveal, the slow peek, fast retract and show after a settled peek. On 2026-11-09 one of 15 runs failed with an uncaptured message; the next 14 passed. The original implementation reproduced the stuck magnification before the fix.

Verified after the animation update: full `make test`, AppKit panel regression (including unsupported clicks during collapse), shutdown regression, signed release build and LaunchAgent restart. Visual quality of glass/refraction is still unverified; native screenshot attempts timed out. P2 permits a display link only while the panel is transitioning; the runtime check confirms it stops after hover, focus reset and visibility animations.

Responsiveness regression: the old easing restarted on every mouse event and reached only scale 1.057 after a 100 ms sweep at 120 Hz. The updated AppKit regression passes a minimum scale of 1.75 after that sweep plus one display interval. Target updates retain spring velocity and elapsed time; only the display-link callback advances and draws animation frames. The pure spring checks exceed 90 % of a fixed step in 80 ms. Focus reset, unsupported clicks, hide/reveal, RGBA conversion, full `make test` and shutdown regression pass after this change. Visual refraction strength remains for the user to judge on the actual desktop.

High-refresh verification: the 60 Hz timer produced 11 distinct geometry states in 180 ms on the current 120 Hz primary display; the CADisplayLink version produced 22 and passed the cadence regression plus focus/click/hide/reveal checks. Window resizing avoids synchronous display calls and skips unchanged bounds. The link is bound directly to NSScreen so a fully transparent/offscreen view does not stop show/hide callbacks; screen changes replace it when needed. In this session, system_profiler reports the main display as built-in Color LCD at 1728×1117 @120 Hz; no 240 Hz display was present, so 240 Hz performance is not observed. The user confirmed the earlier glass was visible; the user found the exposed edge made no visible improvement, so its extra inset was removed; the lighter tint remains.

Notch regression: the original AppKit panel overlapped the actual camera exclusion (`−93…92` pt relative to screen center). The new layout passes that check and returns no control for the gap. Core sweeps check both wings against the exclusion and screen bounds at widths 1512/1728 pt; a 900 pt case checks the compact fallback. The full AppKit focus/hover/cadence/hide/reveal checks and `make test` pass with the actual cutout. The symmetric window canvas keeps screen-relative coordinates and strip input correct when only one wing is magnified.
