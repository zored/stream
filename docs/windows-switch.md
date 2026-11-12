# Window switcher

Controls: [controls.md](controls.md#тайл-ряд-2-колонка-3--window-switcher). Module `switcher` of `zored_stream`, port 47132.
Not to be confused with [windows.md](windows.md), the Windows port.

## Flow

- `POST /switch/tile` (тайл ряд 2, колонка 3): closed → open for 4 s; open → close.
- Left strip = apps, right strip = windows of the picked app. A pick is the item in the ring once its strip stands still
  (no finger, no coasting, the snap to the item is over, ~250 ms after the last knob detent): a new app is activated, then its
  windows are read; a new window is raised. Opening raises nothing: the top app is the frontmost one. Without the wait a fast
  turn activated every app on the way, 7 a second, and the desktop stuttered (verified in the log).
- Input comes the way the emoji picker gets it ([emoji.md](emoji.md)): `pickers` in `picker.go` offers every knob turn, strip
  drag and touch edge to the emoji picker, then to the switcher; `modes.go` keeps at most one of them open. Both draw into the
  same strip layers (`emoji_l`, `emoji_r`, shown by `emoji_on`, owned by `stripOverlay`); the strip physics (`emojiStrip`) is shared.
- One goroutine (`work`) makes every AX call; the job queue holds one job, the newest wins.

## macOS mechanism

- Apps: owners of layer-0 windows from `CGWindowListCopyWindowInfo`, regular activation policy only; the frontmost app first
  (`ax_front_pid`), then the on-screen z-order, then the rest (other Spaces, Stage Manager sets). No Screen Recording: the
  list gives no window names without it (verified: 52 windows, 0 names), and they are not needed here.
- Windows: AX `AXWindows` of the picked app, subrole `AXStandardWindow` or `AXDialog`, title from `AXTitle` (empty → the app
  name on the tile). Read only for the picked app: 10–90 ms per app, 415 ms for all 11 (verified). The elements are kept until
  the next read, so the right strip keeps its order while raises reorder the app's windows.
- Raise: `AXMinimized = false`, the private `_SLPSSetFrontProcessWithOptions(psn, window id, user generated)` (window id from
  `_AXUIElementGetWindow`, 0 for a whole app), then `AXRaise`. Measured from a background process: the window is on top after 57–147 ms.
  The public `NSRunningApplication.activate` is not used: it only asks, and macOS 14+ may ignore a background process (verified:
  it returned YES and the app stayed behind; after Enpass took focus with secure input on, nothing else came forward), and it was slower (220–360 ms).
- Icons: `NSRunningApplication.icon` as a PNG, already turned for the device orientation, cached per pid and turn
  (250 ms for 11 icons cold, verified).
- Permission: the daemon's Accessibility grant (A11).
- Not verified: minimized and full-screen windows, windows on another Space (assumed: activation scrolls to that Space).

## Orientation

As in the emoji picker: icons are turned to stand upright, the left strip is mirrored when the fork flips it, knob turns are
inverted when the device is turned; the orientation file is read on every open. The title card of the right strip and the tiles are turned by the fork only (A12), like the calendar card.

## Rendering

- 60 Hz ticker only while open; a strip PNG is rendered only when its offset or length changes (P3).
- Left strip: 30 pt icons, 43 pt pitch, gray ring on the pick. Right strip: one card, the title of the window at the rounded
  offset, lowercase, JetBrains Mono 15, two lines along the strip, drawn as the calendar list card (`calPNG` rotated); redrawn
  only when the title changes; drag and knob still step one window per 43 pt / detent.
- Tile open: the picked app's icon, 88 pt (6 % inset), redrawn when the app changes.
- Tile idle: the first 4 apps of the switcher order (frontmost, then on-screen z-order) as a 2×2 grid of 40 pt icons; a white
  dot with a black edge on the top-right of an app whose Dock item has a badge (`AXStatusLabel`, matched by app name).
  `idle` runs every 2 s while closed: the Dock read (~0.6 ms) every time, the app list (`CGWindowListCopyWindowInfo`, ~4 ms)
  only when the frontmost pid changed or after the switcher was open; the tile is re-rendered only when the pids, names or
  badges change (verified: 3–4 ms list, 0.6 ms Dock, test process). Not verified: a badge on the device.
  Vars: `switch_tile` plus the shared `emoji_on`, `emoji_l`, `emoji_r`.
