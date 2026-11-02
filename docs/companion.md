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

- Strip cells are 60×90 px (non-square buttons), tap = press in the default "buttons" strip mode.
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
  matches the long-press in `gen_pages.py`). `hapticHoldControls` limits the second one to listed control ids
  (Makefile `HAPTIC_HOLD_CONTROLS`, now `encoder-1-0` = the TRACK knob; knob ids: `encoder-<row>-<0|7>`, buttons `button-1…8`). The driver has `vibrate(pattern)`, upstream never calls it. Patterns: `LoupedeckVibratePattern` in `@loupedeck/core`.
- touch position in buttons mode: sliding a strip updates the fader value variables, upstream only did that in slider mode.
- `hapticSlidePattern`, `hapticSlideStep`: a tick each time a touch crosses a `step`-px line on a strip (270 px tall), at most one per 30 ms.
- left strip swipe → `POST zored_stream /volume/drag?dy=<px>` (`:47113`); right strip → `POST /ai/slide?y=<px>` (`:47117`, absolute position).

The changes stay in the fork: no upstream PR (bitfocus/companion-surface-loupedeck#36 was opened and closed).
