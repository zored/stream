# Window switcher

Controls: [controls.md](controls.md#тайл-ряд-2-колонка-3--window-switcher). Module `switcher` of `zored_stream`, port 47132.
Not to be confused with [windows.md](windows.md), the Windows port.

## Flow

- `POST /switch/tile` (тайл ряд 2, колонка 3): closed → open for 4 s; open → close.
- Left strip = apps, right strip = windows of the picked app. A pick is the item in the ring once its strip stands still
  (no finger, no coasting; a knob detent counts at once): a new app is activated, then its windows are read; a new window is
  raised. Opening raises nothing: the top app is the frontmost one.
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
- Raise: `AXMinimized = false`, `AXRaise`, `NSRunningApplication.activate`. Measured from a background process: the call
  returns in 2–73 ms, the app has focus after 11–120 ms, the window is on top after 220–315 ms. The private
  `_SLPSSetFrontProcessWithOptions` + `AXRaise` put the window on top in 57–106 ms; not used, the public pair is fast enough to follow a knob.
- Icons: `NSRunningApplication.icon` as a PNG, already turned for the device orientation, cached per pid and turn
  (250 ms for 11 icons cold, verified).
- Permission: the daemon's Accessibility grant (A11).
- Not verified: minimized and full-screen windows, windows on another Space (assumed: activation scrolls to that Space).

## Orientation

As in the emoji picker: icons are turned to stand upright, the left strip is mirrored when the fork flips it, knob turns are
inverted when the device is turned; the orientation file is read on every open. The dots of the right strip need no turn.

## Rendering

- 60 Hz ticker only while open; a strip PNG is rendered only when its offset or length changes (P3).
- Left strip: 30 pt icons, 43 pt pitch, gray ring on the pick. Right strip: one white dot per window, the pick in the ring.
- Tile: idle = Bootstrap `window-stack`; open = the picked window's title, JetBrains Mono 15, 6 % inset, wrapped by character.
  Vars: `switch_tile` plus the shared `emoji_on`, `emoji_l`, `emoji_r`.
