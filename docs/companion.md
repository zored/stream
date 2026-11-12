# Bitfocus Companion with the Razer Stream Controller

Verified on Companion 5.1.1, Loupedeck surface module 1.1.1, RSC firmware 0.2.14, macOS 26.

## Install and connect

- macOS: `brew install --cask companion`. Windows: installer from [releases](https://github.com/bitfocus/companion/releases).
  First run asks for interface/port → web UI at `http://127.0.0.1:8000`.
- The official Loupedeck app must be closed: one process owns the serial port. On macOS quitting it also stops `LogiPluginService`.
- Surfaces are modules since 4.3: Surfaces → Add Surface Integration → **Loupedeck** (store). The RSC shows up as `loupedeck:<serial>`.
  The community split-strips surface isn't in the store.
- Known module bug: after unplug/replug the device may need the integration disabled/enabled
  ([companion-surface-loupedeck#30](https://github.com/bitfocus/companion-surface-loupedeck/issues/30)).
- Settings → Protocols: HTTP API on (default). Shell commands (`exec` action) need "shell command support" enabled.

## Grid

4 rows × 8 columns, `row/column` (from `@loupedeck/node` `models/razer-stream-controller.ts`):

| | col 0 | col 1 | cols 2–5 | col 6 | col 7 |
|---|---|---|---|---|---|
| rows 0–2 | left knobs | left strip cell | touch tiles 80×80 | right strip cell | right knobs |
| row 3 | 8 RGB buttons, cols 0–7 | | | | |

- Strip cells are 60×90 px on the grid, but the module draws only 48×86 of each (`SideStripXPadding`/`SideStripYPadding` = 6 px
  hidden on every side, read in the fork's `util.ts`); a picture meant for a whole strip is 48×258, or it gets stretched.
  Tap = press in the default "buttons" strip mode.
- Center tiles: the fork pins a touch to its start tile and posts one swipe per touch to the daemon ([device.md](device.md#touch)).
- Knobs have no display; rotation fires only on a button with `options.rotaryActions: true`.
- RGB button LED colour = the button's background colour (black = off).
- Empty cells are rendered as placeholders: fill every cell with a black button.
- Page navigation buttons don't wrap past the last page.

## Button format (export version 12)

`page.controls[row][col]` → `{type: "button-layered", options, style: {layers}, feedbacks, steps, localVariables}`.

- Every property is `{value, isExpression}`. `isExpression: true` evaluates it: `$(custom:x)`, `$(internal:x)`, math, `max`/`min`,
  ternaries, string literals (`$(custom:np_playing) == 1 ? "⏸" : "▶"`). Works for text, colour, opacity, x/y/width/height, rotation, enabled.
- Layers: `canvas` (`decoration: none` hides the top bar with `1/r/c`, `showStatusIcons: none` hides the green "running" triangle),
  `box`, `text`. Geometry is % of the cell and may go outside 0–100: content is clipped, which lets one layer span several cells.
- `options.canModifyStyleInApis: true` is required for the legacy HTTP style API to touch the button.
- `steps["0"].action_sets`: `down`, `up`, `rotate_left`, `rotate_right`. Actions in a set run in parallel.
- Long press: a numeric set (`"400"`) listed in `steps["0"].options.runWhileHeld` fires once the control is held that long;
  `up` then runs only on a shorter press. `gen_pages.py`: `button(..., up=..., held=...)`.
- Action entity: `{type: "action", id, connectionId: "internal", definitionId, options, upgradeIndex: null}`. Useful internal ones:
  - `exec` — options `path` (shell command, `$HOME` expands), `cwd`, `timeout` 500–20000, `targetVariable` (custom var gets stdout)
  - `custom_log` — `message`, written to the log; the cheapest way to prove an input arrives
  - `custom_variable_set_value` — `name`, `create` (create if missing), `value` (may be an expression)
  - `set_brightness` — `surfaceId` (`self`), `brightness` 0–100 (expression ok); `adjust_brightness` ± — saved in the surface config
  - `action_group` — `execution_mode` `sequential`, `children.default` = actions; `exec` is awaited, so a following action sees its `targetVariable`
- Internal variables: `time_hm`, `time_hms`, `time_unix` (updates every 0.5 s), `date_d`, `date_m`, `date_y`, `date_weekday`, …

`tools/gen_pages.py` builds all of this; read it before hand-writing JSON.

## APIs

| Need | How | Auth |
|---|---|---|
| Press / rotate a control | `POST /api/location/<page>/<row>/<col>/press` (`down`, `up`, `rotate-left`, `rotate-right`) | none |
| Set a custom variable | `POST /api/custom-variable/<name>/value`, body = JSON value (`"text"`) | none |
| Read a variable | `GET /api/custom-variable/<name>/value`, `GET /api/variable/internal/<name>/value` | none |
| Export a page | `GET /int/export/page/<n>?format=json\|yaml` | none |
| Import a page / create custom variables | internal tRPC, see below | none |
| Connections, surfaces | REST API v2 `/api/v2` (OpenAPI at `/api/v2/openapi.json`) — no buttons in 5.1.1 | bearer key, Settings → Protocols |

- A text/plain body that is empty is rejected with `400 No value`; send JSON (`""`).
- tRPC: WebSocket `ws://127.0.0.1:8000/trpc`, messages `{id, method: "mutation", params: {path, input}}`, no transformer.
  Page import must happen in one connection (state lives in the socket context):
  `importExport.prepareImport.start {name, size}` → `uploadChunk {sessionId, offset, data: base64}` →
  `complete {sessionId, expectedChecksum: sha1hex, userData: null}` → `importExport.importSinglePage {targetPage, sourcePage: 1, connectionIdRemapping: {}}` → `importExport.abort`.
  `customVariables.create {name, defaultVal}` returns the string `Variable "<name>" already exists` instead of throwing.
  `tools/deploy.mjs` implements both.
- Surfaces: `instances.surfaces.setModuleAndVersion {instanceId, moduleId, versionId: "dev"}`;
  `surfaces.surfaceSetConfigKey {surfaceId, key: "plugin_cfg_<field>", value}` (only while the surface is open).
  Ids come from the subscriptions `instances.surfaces.watch` and `surfaces.watchSurfaces` (first message `{type: "init", info}`).
  `tools/surface.mjs` implements it.
- The router lives in Companion's `main.js` (`Resources/main.js` in the app bundle); grep it for procedure names when something new is needed.

## Debugging

- Log: Log page, or `~/Library/Application Support/companion/logs/` (macOS). With debug logging on, inputs appear as
  `Surface/Handler/loupedeck:<serial> Button <page>/<row>/<col> pressed`.
- Web previews of strip cells are square; the editor can't show the real 60×90 aspect. Check strips on the device.
- Raw device traffic without Companion: quit Companion, run `tools/sniff.py` (see [device.md](device.md)).

## Fonts in SVG images

Image layers go through `@napi-rs/canvas` (Skia) `loadImage`; verified by rendering with the bundled module and node runtime.
- `font-family` resolves system fonts and `~/Library/Fonts`; an unknown family falls back silently. `@font-face` is ignored.
- Variable fonts render only their default instance (`font-weight` ignored): install static weights.
- `letter-spacing` is ignored; narrow text with `transform="scale(x 1)"`.
- Device text uses JetBrains Mono (OFL, static weights in `~/Library/Fonts`): clock, date, track strip (scaled to 85 %).

## Animation frame rate

Measured 2026-11-02 with temporary counters in the module fork (draw calls, bytes, time from `DrawFramebuffer` to its ack)
and a script posting custom variables over keep-alive HTTP (≈1.5 ms per POST). Verified unless marked.

Pipeline: daemon → `POST /api/custom-variable` → Companion re-renders every button that references the variable
(debounce `wait 10 ms, maxWait 20 ms` per button, worker-thread render, LRU cache keyed by the style JSON, identical
images are not re-sent) → module `draw()` → `@loupedeck/core` draw queue (one `p-queue`, concurrency 1, for the
whole device; each draw waits for the device ack) → serial.

| Input rate | Right strip, one SVG over 3 cells | Tile 1/2 |
|---|---|---|
| 30 Hz | 28 fps per cell, 84 draws/s | 30 fps |
| 60 Hz | **60 fps** per cell, 180 draws/s | 60 fps |
| 90 Hz | 42–52 fps | 25 fps¹ |
| 120 Hz | 40 fps | 38 fps¹ |

¹ The tile test toggles between two images; above ~50 Hz two toggles inside one debounce window cancel out,
so tile numbers above 60 Hz undercount. The strip test changes every frame and is the reliable one.

- Device: 1.3–2 ms per draw for a 48×86 strip cell (8 KB RGB565) or an 80×80 tile (12.8 KB), ack included,
  flat across all rates. At 60 fps on a full strip the serial queue is busy ~33 % of the time.
  Ceiling ≈ 500 draws/s ≈ 180 fps for a full strip (assumed, extrapolated; the ack may come before the panel shows it).
- Companion is the limit: past ~60 Hz input its trailing debounce keeps restarting and fewer frames come out.
  60 Hz (a frame every 16.7 ms, longer than the 10 ms wait) is the sweet spot; also +10 ms latency per frame.
- The daemon is not: `vars.set` skips unchanged values, so an easing tail sends fewer posts (seen: 14/s at `stripHz` 30).
- A strip picture spans 3 buttons, so it costs 3 renders and 3 draws per frame; a per-cell layout redraws only cells that change.

Options, cheapest first:

1. Done: `animationHz`/`stripHz` 30 → 60 in the daemon, easing factors rescaled per frame (`k60 = 1 − √(1 − k30)`) so
   durations stay the same. On the device 60 Hz looks smoother than 30 (user, left strip); posts arrive every 16.7 ms. Short bursts only
   (the tickers already stop when the animation settles), so idle cost stays zero.
2. Redraw only what moves: keep static parts in separate layers/cells so fewer buttons re-render per frame.
3. Bypass variables: draw animations in the module fork itself (endpoint or IPC with a target value, the fork eases
   and calls `drawBuffer` on a dirty rectangle). Skips Companion's debounce and render, could reach 100+ fps (assumed),
   but moves drawing code into the fork.

## Loupedeck surface module fork

Companion runs [zored/companion-surface-loupedeck](https://github.com/zored/companion-surface-loupedeck/tree/zored_stream),
branch `zored_stream`, cloned next to this repo. `make module` builds it and loads it as a developer module (verified on 5.1.1):

- The launcher's `config.json` (`~/Library/Application Support/companion/`) has `enable_developer` and `dev_modules_path`;
  with both set it starts the core with `--extra-module-path`. Read at launch, so Companion restarts after a change.
- Every subdirectory with `companion/manifest.json` is a module, version `dev`; `$(SHARE)/companion-modules/loupedeck` is a symlink to the fork.
  Not packaged (has `@companion-surface/base` in `package.json`), so the entrypoint is `dist/main.js` and the API version comes from `node_modules`.
- The Loupedeck integration must be switched to `dev` (`tools/surface.mjs`); the log then shows `Surface: loupedeck: Loupedeck (Dev)`.
- Companion watches the dev path: `yarn build` in the fork reloads the module without a restart (assumed, from the launcher code).

Fork commits (surface settings, `plugin_cfg_*`):

- `hapticPressPattern`, `hapticHoldMs`: haptic on button/knob/tile press and a second one after the hold time (400 ms here,
  matches the long-press in `gen_pages.py`). `hapticHoldControls` limits the second one to listed control ids (generated `companion/hold-controls.txt`)
  (Makefile `HAPTIC_HOLD_CONTROLS`, now `encoder-1-0` = the TRACK knob; knob ids: `encoder-<row>-<0|7>`, buttons `button-1…8`). The driver has `vibrate(pattern)`, upstream never calls it. Patterns: `LoupedeckVibratePattern` in `@loupedeck/core`.
- touch position in buttons mode: sliding a strip updates the fader value variables, upstream only did that in slider mode.
- left strip swipe → `POST zored_stream /volume/drag?dy=<px>` (`:47113`); right strip → `POST /ai/slide?y=<px>` (`:47117`, absolute position).
- orientation (`src/orientation.ts`): holding `encoder-0-7` 3 s without a turn, then every 2 s while held, steps 90 → 0 → 270 → 180 → 90 (device turned clockwise; each step turns the picture 90° clockwise),
  saved in `~/.config/zored-stream/orientation` (missing = 90, anything else = error). Tile buffers turn the other way before
  `drawKeyBuffer`; the strip that ends up upside down (left at 90, right at 270, both at 180) gets each cell turned 180° and drawn in the
  mirrored slot, and its touch y becomes `270 − y` (cells, fader variables, `/volume/drag`, `/ai/slide`, `/emoji/*`); tile swipe
  directions turn with the device. Cached images are redrawn on a switch. Pages, the daemon and the screen panel always draw for 0°.
  Companion's own Surface Rotation (5.1.1: `surface90`/`surface-90`/`surface180`) remaps the grid 4×8 → 8×4 and turns strips
  like tiles, so it stays off.
- sleep moon (`src/moon.ts`): `close()` blanks the device, then draws a white crescent on the tile at row 0, column 5 (тайл ряд 1, колонка 4); Companion closes surfaces on system sleep, so the moon stays while the Mac sleeps.

The changes stay in the fork: no upstream PR (bitfocus/companion-surface-loupedeck#36 was opened and closed).
