# Screen panel

An on-screen stand-in for the controller on macOS while the device is unplugged. Decided with the user 2026-11-06.
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

- One row at the top center of the main screen, above all windows, on all Spaces, never takes focus (`NSPanel`, `.nonactivatingPanel`, level `.statusBar`).
- 12 tiles ordered by device column: 4 groups of 3. Group *c* holds тайл ряд 1–3, колонка *c*. Tile number *n* = 1…12 in that order.
- Strips lie horizontally at both ends of the row, bottom edge towards the tiles: the left strip turned 90° counterclockwise
  (device top → screen left), the right strip 90° clockwise (device top → screen right).
- No кнопки 1–8 on screen.
- Small mode: tile 40 pt, strip 40 × 215 pt (48 × 258 turned), rounded corners (tile 9 pt, strip 8 pt), black, hairline outline.
- Hover: Dock-style magnification, each element by its own distance to the cursor (cosine falloff, range 200 pt),
  up to 80 pt per tile (= the device tile); grows downwards from the top edge.
- Style follows the device (D1–D3): the pictures are Companion's, the app adds only the outline and corners.

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
| cursor at the top screen edge (≤ 2 pt) while hidden | show the panel again |

Strip mouse position → device y (0 … 270, 0 = top): left strip `y = 270 · x / width`, right strip `y = 270 · (1 − x / width)`.

## Frames

Satellite `ADD-DEVICE` with `LAYOUT_MANIFEST` (verified in Companion 5.1.1: per-control `bitmap {w, h}` style presets,
`KEY-ROTATE` by `CONTROLID`): the 4×8 grid of the Loupedeck page, presets `tile` 160×160 and `strip` 120×180
(2× the largest on-screen size). The surface joins the Loupedeck's surface group so it shows the same page (assumed: set once through tRPC like `tools/surface.mjs`).
Companion pushes a frame only when a picture changes, through the same debounce as the device: ≤ 60 fps (assumed: same pipeline).
Each control is one `CALayer`; a strip is three cells cropped by 6 px (the module's hidden padding) into 48×258, then turned.

## When it shows

- Device plugged (IOKit USB match VID `0x1532`, PID `0x0D06`) → panel hidden, Satellite disconnected: Companion renders nothing extra.
- Unplugged → connect, show.
- Hidden by a double click → stays hidden until the cursor touches the top edge; a global `mouseMoved` monitor runs only while hidden
  (assumed: needs no Accessibility).
- Idle CPU ≈ 0: no timers; frames and mouse events drive everything (P2).

## Open

- macOS reveals the auto-hidden menu bar on the same top-edge move; both show, check how it looks.
- The main display is an OLED (MPG321UX): static white pixels all day. A 1–2 px drift every few minutes if the user wants it.
- iOS later: a PWA speaking Satellite over WebSocket `:16623`, or SwiftUI sharing `ScreenCore`; a paid Apple Developer account
  past the 7-day free signing. Satellite listens on `*` (verified), so the LAN reaches it.
