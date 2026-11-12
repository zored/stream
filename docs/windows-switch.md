# Window switcher

Controls: [controls.md](controls.md#тайл-ряд-2-колонка-3--window-switcher). Module `switcher` of `zored_stream`, port 47132.
Not to be confused with [windows.md](windows.md), the Windows port.

## Flow

- `POST /switch/tile/down` (тайл ряд 2, колонка 3): closed → open for 4 s; open → a press starts. `POST /switch/tile/up` ends it.
  Knob presses (`/volume/mute` + `/volume/release`, `/brightness/hold` + `/brightness/release`) start and end presses the same way.
- A press is timed in the daemon: released before 0.4 s = click (commit: raise the window in the right ring, or activate the app
  in the left ring when its windows are not loaded or it has none, then close); at 0.4 s held = close the window in the right ring
  (`AXCloseButton` → `AXPress`, the list is read again, the ring keeps its index); 2 s later = `NSRunningApplication.terminate`
  of the picked app, removed from the left strip at once. A held press ends without a click; the switcher stays open.
  Companion's own hold set is not used: its release set would not run after a hold, and the volume / brightness knobs need it.
- Left strip = apps, right strip = windows of the picked app. Once the left strip stands still (no finger, no coasting, the
  snap is over) the windows of its app are read; nothing is raised. Only a tap raises: a left tap activates the tapped app once
  the strip has brought it to the ring, a right tap raises the ring window. Earlier versions raised every settled pick; a fast
  turn activated every app on the way and the desktop stuttered (verified in the log).
- Input comes the way the emoji picker gets it ([emoji.md](emoji.md)): `pickers` in `picker.go` offers every knob turn, strip
  drag and touch edge to the emoji picker, then to the switcher; `modes.go` keeps at most one of them open. Both draw into the
  same strip layers (`emoji_l`, `emoji_r`, shown by `emoji_on`, owned by `stripOverlay`); the strip physics (`emojiStrip`) is shared.
- One goroutine (`work`) makes every AX call, in order; the queue holds 4 jobs, a full queue drops the oldest.

## macOS mechanism

- Apps: owners of layer-0 windows from `CGWindowListCopyWindowInfo`, regular activation policy only; the frontmost app first
  (`ax_front_pid`), then the on-screen z-order, then the rest (other Spaces, Stage Manager sets), then running regular apps with no window
  (`NSWorkspace.runningApplications` order). No Screen Recording: the
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
- Left strip: 30 pt icons, 43 pt pitch, gray ring on the pick. Right strip: the window titles as lines along the strip
  (`calPNG` rotated, JetBrains Mono 15, line 18.75 pt), the ring line white and vertically centred with the next one, the others
  gray, shifted by the fractional offset so a scroll slides; redrawn when the offset changes (≤ 60 Hz); drag and knob step one
  window per 43 pt / detent.
- Chevrons: knob hints `l0` and `r0` are owned by the open switcher ([knobhint.go](../tools/zored_stream/knobhint.go)).
- Tile open: the picked app's icon, 88 pt (6 % inset), redrawn when the app changes.
- Tile idle: the first 4 apps of the switcher order (frontmost, then on-screen z-order) as a 2×2 grid of 40 pt icons; a white
  dot with a black edge on the top-right of an app whose Dock item has a badge (`AXStatusLabel`, matched by app name).
  `idle` runs every 2 s while closed: the Dock read (~0.6 ms) every time, the app list (`CGWindowListCopyWindowInfo`, ~4 ms)
  only when the frontmost pid changed or after the switcher was open; the tile is re-rendered only when the pids, names or
  badges change (verified: 3–4 ms list, 0.6 ms Dock, test process). Not verified: a badge on the device.
  Vars: `switch_tile` plus the shared `emoji_on`, `emoji_l`, `emoji_r`.
