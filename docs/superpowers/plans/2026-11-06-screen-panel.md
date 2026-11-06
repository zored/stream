# Screen Panel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** An always-on-top row of the controller's 12 tiles and 2 strips at the top of the Mac screen while the device is unplugged.

**Architecture:** A Swift AppKit app in `tools/screen/` registers with Companion as a Satellite surface, shows the frames Companion pushes
in one `NSPanel`, and turns mouse input into `KEY-PRESS` / `KEY-ROTATE` and the same daemon posts the Loupedeck fork makes.
Pure logic (order, mapping, geometry, protocol lines) lives in `ScreenCore`, checked by an executable target.

**Tech Stack:** Swift 6.4 (Command Line Tools), AppKit, Core Animation, Network.framework (TCP), IOKit; Make + LaunchAgent.

**Spec:** `docs/screen.md`

## Global Constraints

- Companion Satellite TCP `127.0.0.1:16622`; device ids `DEVICEID=zored-screen`, `PRODUCT_NAME="zored screen"`.
- Daemon URLs exactly as the fork: `http://127.0.0.1:47113/volume/drag?dy=`, `:47117/ai/slide?y=`, `:47128/emoji/{start,end}?s=&y=`, `:47113/tile/swipe?id=&dir=`; every request has a 1 s timeout.
- No new port: the app serves nothing (A2 unchanged).
- Grid ids `row/col` from `docs/companion.md`: knobs `r/0`, `r/7`; strips `r/1`, `r/6`; tiles `r/2…5`; кнопки `3/0…7`.
- Small tile 40 pt, strip 40×215 pt, max tile 80 pt, magnification range 200 pt, swipe threshold 40 pt within 400 ms.
- Fail loudly (A9): Satellite `ERROR` replies, bad manifest, lost connection → log line + exit, `KeepAlive` restarts.
- Signed `codesign -s zored-stream -i com.zored.stream.screen` (A11 pattern).
- `swift test` does not work with Command Line Tools here (verified 2026-11-06: `error: fatalError`); checks are `swift run screen-checks`.
- Commit: `git -C /Users/r.akhmerov/git/github.com/zored/stream commit -m main -- <paths>`, then push.

## Review Focus

- Companion restarts while the panel is up → the app exits, launchd restarts it, it reconnects (pinned in Task 3).
- Mouse released outside the tile it went down on → the up still goes to the down's control (pinned in Task 4, F2-like).
- Device plugged mid-drag on a strip → panel hides, no stray `down` left pressed in Companion (pinned in Task 5).
- Double click on a strip also emits two emoji start/end pairs → harmless while the picker is closed (F22); checked by hand in Task 6.
- Panel on a display that is not the main one / display unplugged → panel moves to the main screen on `didChangeScreenParameters` (Task 4).

---

### Task 1: Package and `ScreenCore` layout

**Files:**
- Create: `tools/screen/Package.swift` (targets `ScreenCore` library, `screen` executable, `screen-checks` executable)
- Create: `tools/screen/Sources/ScreenCore/Layout.swift`
- Create: `tools/screen/Sources/screen-checks/main.swift`

**Interfaces:**
- Produces: `struct GridID: Hashable { row: Int; col: Int; var id: String /* "r/c" */ }`;
  `enum Element { case tile(n: Int, grid: GridID); case strip(Side) }`, `enum Side { case left, right }`;
  `Layout.tiles: [GridID]` (12, column-major); `Layout.knob(forTile n: Int) -> GridID?`; `Layout.button(forTile n: Int) -> GridID?`;
  `Layout.stripCells(_ side: Side) -> [GridID]`.

- [ ] **Step 1: Write checks** in `screen-checks/main.swift` with a `check(_ cond: Bool, _ msg: String)` that exits 1:
  `Layout.tiles.map(\.id) == ["0/2","1/2","2/2","0/3","1/3","2/3","0/4","1/4","2/4","0/5","1/5","2/5"]`;
  `knob(forTile: 1) == GridID(0,0)`, `knob(forTile: 3) == GridID(2,0)`, `knob(forTile: 10) == GridID(0,7)`, `knob(forTile: 12) == GridID(2,7)`, `knob(forTile: 5) == nil`;
  `button(forTile: 1) == GridID(3,0)`, `button(forTile: 8) == GridID(3,7)`, `button(forTile: 9) == nil`;
  `stripCells(.left).map(\.id) == ["0/1","1/1","2/1"]`, `stripCells(.right).map(\.id) == ["0/6","1/6","2/6"]`.
- [ ] **Step 2:** `cd tools/screen && swift run screen-checks` → fails to compile (no `Layout`).
- [ ] **Step 3:** Implement `Layout.swift`.
- [ ] **Step 4:** `swift run screen-checks` → prints `ok`, exit 0.
- [ ] **Step 5:** Commit `tools/screen/Package.swift tools/screen/Sources`.

### Task 2: Geometry — strip coordinates and magnification

**Files:**
- Create: `tools/screen/Sources/ScreenCore/Geometry.swift`
- Modify: `tools/screen/Sources/screen-checks/main.swift`

**Interfaces:**
- Produces: `Geometry.deviceY(side: Side, x: Double, width: Double) -> Int` (0…270, clamped);
  `Geometry.scale(distance: Double, small: Double, max: Double, range: Double) -> Double` (cosine falloff, 1 at ≥ range);
  `Geometry.swipe(dx: Double, dy: Double, ms: Double) -> String?` (`"l"|"r"|"u"|"d"`, nil under 40 pt or over 400 ms).

- [ ] **Step 1: Checks:** `deviceY(.left, x: 0, width: 215) == 0`, `deviceY(.left, x: 215, width: 215) == 270`,
  `deviceY(.right, x: 0, width: 215) == 270`, `deviceY(.right, x: 215, width: 215) == 0`, `deviceY(.left, x: -5, width: 215) == 0`;
  `scale(distance: 0, small: 40, max: 80, range: 200) == 2.0`, `scale(distance: 200, …) == 1.0`, `abs(scale(distance: 100, …) - 1.5) < 1e-9`;
  `swipe(dx: 50, dy: 3, ms: 100) == "r"`, `swipe(dx: -3, dy: -45, ms: 100) == "u"`, `swipe(dx: 30, dy: 0, ms: 100) == nil`, `swipe(dx: 60, dy: 0, ms: 500) == nil`.
- [ ] **Step 2:** `swift run screen-checks` → compile error.
- [ ] **Step 3:** Implement. Screen y grows down, so `dy < 0` is `"u"`.
- [ ] **Step 4:** `swift run screen-checks` → `ok`.
- [ ] **Step 5:** Commit.

### Task 3: Satellite client

**Files:**
- Create: `tools/screen/Sources/ScreenCore/Satellite.swift` (line codec, pure)
- Create: `tools/screen/Sources/screen/SatelliteConnection.swift` (`NWConnection` TCP)
- Modify: `tools/screen/Sources/screen-checks/main.swift`

**Interfaces:**
- Produces (pure): `Satellite.manifest() -> String` (base64 JSON: controls for all 32 grid ids, `stylePreset` `tile` / `strip` / `none`,
  presets `default {}`, `tile {bitmap {w:160,h:160}}`, `strip {bitmap {w:120,h:180}}`; knobs and кнопки get `default`);
  `Satellite.addDevice() -> String`; `Satellite.press(_ id: GridID, down: Bool) -> String`; `Satellite.rotate(_ id: GridID, right: Bool) -> String`;
  `Satellite.parse(_ line: String) -> Message` with `enum Message { case keyState(GridID, Data /* raw RGB */); case ping; case ok; case error(String); case other }`.
- Produces (app): `final class SatelliteConnection { var onFrame: (GridID, CGImage) -> Void; func press(_:down:); func rotate(_:right:) }`.

- [ ] **Step 1: Checks:** decoded manifest JSON has 32 controls, `controls["0/1"].stylePreset == "strip"`, `controls["1/3"].stylePreset == "tile"`;
  `press(GridID(0,2), down: true) == "KEY-PRESS DEVICEID=zored-screen CONTROLID=0/2 PRESSED=true\n"`;
  `rotate(GridID(1,0), right: false) == "KEY-ROTATE DEVICEID=zored-screen CONTROLID=1/0 DIRECTION=-1\n"`;
  `parse("PING 1") == .ping`; `parse("KEY-STATE DEVICEID=zored-screen CONTROLID=0/2 BITMAP=AAAA") == .keyState(GridID(0,2), Data([0,0,0]))`.
  Before writing the exact field names, read Companion's satellite handler (`grep -o '#u(e,r){.\{600\}' main.js` for KEY-PRESS, the `KEY-STATE` sender) and copy them; record them in `docs/screen.md`.
- [ ] **Step 2:** `swift run screen-checks` → compile error.
- [ ] **Step 3:** Implement the codec, then the connection: answer `PING` with `PONG`, `BEGIN` → send `addDevice()`, `ERROR` or a closed socket → log + `exit(1)`.
- [ ] **Step 4:** `swift run screen-checks` → `ok`; `swift run screen --probe` (connects, logs every `KEY-STATE` id for 5 s, exits) shows all 12 tile ids and 6 strip ids. Device must be unplugged or Companion must accept a second surface: check both.
- [ ] **Step 5:** Put the satellite surface into the Loupedeck's group on page 1 (tRPC in `tools/surface.mjs`, new `group` command); confirm in Companion's Surfaces page.
- [ ] **Step 6:** Commit.

### Task 4: Panel, layers, mouse

**Files:**
- Create: `tools/screen/Sources/screen/Panel.swift` (NSPanel + one `CALayer` per element, magnification)
- Create: `tools/screen/Sources/screen/Input.swift` (mouse → `SatelliteConnection` / daemon posts)
- Create: `tools/screen/Sources/screen/Daemon.swift` (`URLSession` posts, 1 s timeout, log on failure)
- Create: `tools/screen/Sources/screen/main.swift`

**Interfaces:**
- Consumes: `Layout`, `Geometry`, `SatelliteConnection`.
- Produces: `final class Panel { func show(); func hide(); var isHidden: Bool; var onStripDoubleClick: () -> Void }`.

- [ ] **Step 1:** Panel per spec: top center of `NSScreen.main`, `.nonactivatingPanel`, `.statusBar`, `collectionBehavior [.canJoinAllSpaces, .fullScreenAuxiliary]`, transparent background. Strip layer = 3 cells cropped 6 px each, composed into 48×258, rotated −90° / +90°.
- [ ] **Step 2:** Magnification on `mouseMoved` inside a tracking area 200 pt larger than the row; layout recomputed with `CATransaction.setDisableActions(true)`.
- [ ] **Step 3:** Input table from `docs/screen.md`: the control pressed on mouse down receives the up whatever the cursor is over; wheel ticks → one rotate per `scrollingDeltaY` step of 1 (trackpad: accumulate 10 pt per step).
- [ ] **Step 4:** Move to the main screen on `NSApplication.didChangeScreenParametersNotification`.
- [ ] **Step 5: Verify:** with the device unplugged, `swift run screen`; in Companion's debug log a click on tile 4 shows `Button 1/0/3 pressed`; wheel over tile 1 moves the volume (`GET /api/custom-variable/volume/value` changes); right click on tile 12 toggles scroll mode; middle click on tile 7 = кнопка 7; drag along the left strip changes the volume.
- [ ] **Step 6:** `top -pid $(pgrep -x screen) -l 7 -s 10` with nothing moving → ≈ 0 %.
- [ ] **Step 7:** Commit.

### Task 5: Device detection, hide / reveal

**Files:**
- Create: `tools/screen/Sources/screen/Device.swift` (IOKit `IOServiceAddMatchingNotification` for VID `0x1532` PID `0x0D06`, matched + terminated)
- Modify: `tools/screen/Sources/screen/main.swift`

**Interfaces:**
- Produces: `final class Device { var onChange: (Bool /* plugged */) -> Void }`.

- [ ] **Step 1:** Plugged → release every held control (send its up), disconnect Satellite, hide. Unplugged → connect, show.
- [ ] **Step 2:** Double click on a strip → hide; a global `mouseMoved` monitor exists only while hidden by the user; cursor `y ≥ screen.maxY − 2` → show, remove the monitor.
- [ ] **Step 3: Verify:** plug the device → panel gone within 1 s and Companion's Surfaces page lists no `zored-screen`; unplug → back. Double click a strip → gone; cursor to the top edge → back. If Accessibility is asked for the global monitor, stop and tell the user.
- [ ] **Step 4:** Commit.

### Task 6: Install, docs, invariants

**Files:**
- Create: `tools/screen/com.zored.stream.screen.plist` (`KeepAlive`, `ThrottleInterval` 5, log `~/Library/Logs/zored-stream/screen.log`)
- Modify: `Makefile` (`screen-bin`: `swift build -c release` + `codesign -f -s $(SIGN) -i com.zored.stream.screen`; `screen`: `$(call install-agent,screen)`; `screen-uninstall`; `test` also runs `cd tools/screen && swift run screen-checks`)
- Modify: `.github/workflows/ci.yml` (build job: `cd tools/screen && swift run screen-checks`)
- Modify: `docs/screen.md` (verified / assumed after the run), `docs/controls.md` (section "On screen"), `docs/invariants.md`
  (A1: a second surface, `tools/screen`, makes the same daemon posts as the fork; P2: the screen app has no timers), `CLAUDE.md` (docs list)

- [ ] **Step 1:** `make screen` → agent running, panel up with the device unplugged.
- [ ] **Step 2:** `make test` → green.
- [ ] **Step 3:** Manual: Review Focus lines 1 (`kill` Companion helper → app restarts and reconnects) and 4 (double click with the emoji picker closed changes nothing).
- [ ] **Step 4:** Commit all task paths, push.
