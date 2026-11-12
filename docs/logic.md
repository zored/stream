# Behaviour spec for porting

What every control and every daemon module does, written so that an agent with no other context can re-implement it on
another OS. Read this before writing any `*_windows.go` file or touching the `windows` page profile.

- **Source of truth**: the code, `tools/gen_pages.py` and `tools/zored_stream/*.go`, as of 2026-11-10. Everything here was
  read in the code unless marked *(assumed)*. Where another doc disagrees with the code, the code wins and the gap is listed in
  [Docs that disagree with the code](#docs-that-disagree-with-the-code).
- **Port plan** (what to keep, drop, install steps, open questions): [windows.md](windows.md). This file does not repeat it;
  it says what the behaviour is, so the plan can be carried out.
- **Rules that must keep holding**: [invariants.md](invariants.md), cited here by number (A1, F5, …).
- **Control names**: "крутилка слева, ряд 1" = left knob, row 1; "тайл ряд 2, колонка 1" = touch tile row 2, column 1;
  "кнопка 3" = third round button. Grid coords `row/col` (0-based) as in [companion.md](companion.md#grid):
  knobs `r/0` and `r/7`, strips `r/1` and `r/6`, tiles `r/2…r/5`, buttons `3/0…3/7`.

## 1. System model

```
device ──serial── Companion (+ Loupedeck module fork) ──exec curl──▶ zored_stream (one Go binary, one HTTP server per port)
                     ▲                                                     │
                     └──────── POST :8000/api/custom-variable/<name>/value ─┘
```

1. **Companion draws and runs actions; the daemon owns all state and every OS call** (A1). Companion never computes state;
   it only renders layers whose properties are expressions over `$(custom:<var>)` and `$(internal:<var>)`.
2. **Companion → daemon**: an `exec` action running `curl -fsS -m 1 -X POST http://127.0.0.1:<port><path>`. `-m 1` is
   mandatory (CI checks it): Companion awaits the exec, a slow daemon must not hang it. `-f` makes a `409` a failed exec
   (used by the choice-mode gate, F1). On Windows `exec` runs through `cmd.exe`, so URLs containing `?` are double-quoted
   (`gen_pages.py` `ai()`); `curl.exe` ships with Windows 10+ *(assumed)*.
3. **Module fork → daemon** (no Companion action involved, OS-independent JS in the fork, A10):
   - left strip slide: `POST :47113/volume/drag?dy=<px>` (relative, positive = down the strip *(assumed sign)*);
   - right strip slide: `POST :47117/ai/slide?y=<px>` (absolute position along the strip);
   - centre tiles swipe: `POST :47113/tile/swipe?id=<control>&dir=l|r|u|d`, once per touch, when the touch moved ≥ 40 px
     from its start within 400 ms ([device.md](device.md#touch)); `404` = this tile has no swipes, the touch goes on as a press;
     anything else = the fork releases the tile right after the answer;
   - emoji strips while the picker is open: `POST :47128/emoji/start?s=l|r&y=<px>`, `POST :47128/emoji/end?s=l|r`.
4. **Daemon → Companion**: `POST http://127.0.0.1:8000/api/custom-variable/<name>/value`, body = the value as a JSON
   string (`"42"`, never an empty text body: Companion answers `400 No value`). HTTP client timeout 1 s.
5. **Variable cache** (`main.go` `vars`, A4): `set(name, value)` posts only when the value differs from the last sent one;
   a failed post forgets the value so the next `set` retries. Every 10 s (`resendEvery`) the cache is cleared and every
   module re-publishes, so a restarted Companion heals within 10 s. The call module keeps its own cache with the same rule.
6. **Route conventions** (`main.go` `handle`): action routes are `POST`, answer `204` on success, `409` + a log line on a
   failed action (A9). `handleValue` routes answer `200` with a plain-text integer (brightness). Bad query parameters → `400`,
   unknown path value → `404`.
7. **Every `POST` except `/agent/event` is user activity** (`serve` → `touched.touch()`, `power.go`): at most once per 5 s
   the daemon tells the OS the user is active (macOS: zero-move mouse event + `IOPMAssertionDeclareUserActivity`). Windows:
   `SetThreadExecutionState(ES_DISPLAY_REQUIRED)` or a zero `SendInput` mouse move *(assumed)*. This lives in `power.go`,
   which is mac-only today; the Windows build must still provide `touched` (see [§6](#6-what-a-windows-build-must-provide)).
8. **Fail loudly** (A9): bad flags, unreadable state files, no audio device at start → `log.Fatal`; the supervisor
   (LaunchAgent `KeepAlive` on macOS, a logon task with restart on Windows) restarts it.
9. **Idle cost ~0** (P2): animation tickers run only while something moves; periodic publishers re-render an image only when
   its frame key changes (`frame.go`).

### Shared primitives (all portable Go)

| Primitive | File | Behaviour |
|---|---|---|
| knob acceleration `accel.step(dir)` | `bar.go` | time since the previous detent in the **same** direction: > 200 ms → 1, > 100 ms → 2, > 50 ms → 3, else 4 (signed). A direction change counts as slow. Used by volume (%), brightness (%), seek (index into 2/4/8/15 s), scroll (index into 1/2/4/8 lines), keep-awake length, emoji |
| left-strip bar `bar` | `bar.go` | one bar shared by volume, display brightness, controller brightness, seek. `show(mode, value)`: target = value, visible for 1 s (`visibleFor`). A hidden bar switching mode jumps to the new value instead of easing. 60 Hz ticker while active: `shown += (target − shown) × 0.163` (snap at < 0.3), alpha → 100 with factor 0.368, → 0 with 0.134 (snap at < 1). Publishes `bar_mode` (`volume`/`mac`/`ctl`/`seek`), `bar_value` (`%.1f`, 0–100), `bar_alpha` (`%.0f`, 0–100) |
| tile layers `slot` | `layers.go` | ordered layers `{name, active(), press(), up()}`; the first active one wins, the last is the default. `publish` sets the slot variable to the winning name every 250 ms; `press` routes to the winner and remembers it, `up` goes to the same layer even if another one won meanwhile (F23) |
| `frame` | `frame.go` | caches a rendered image by a string key; re-renders only when the key changes |
| orientation | `orientation.go`, `stripflip.go`, fork `src/orientation.ts` | the device can stand at 0/90/180/270° (file `~/.config/zored-stream/orientation`, missing = 90, written only by the fork on a 3 s hold of крутилка справа, ряд 1). Everything upstream of the fork draws and reads touches for 0° (A12); the daemon reads the file only for `strip_flip` (left-strip bars mirrored at 90/180) and the emoji glyph turn. Portable as is |
| one mode at a time `tileModes` | `modes.go` | mac only. Modes `{name, active(), close()}`: wi-fi pick, keep-awake length edit, emoji, window switcher, chats browse, weather browse, calendar list, timer edit. After every daemon POST `check()` compares who is active with the last check; if one just opened, every mode that was already open is closed (`close` = its timeout: timer edit commits on the next tick, wi-fi `closePick` drops a pending join). Modes opening in the same request all stay |
| knob hint `knobHints` | `knobhint.go` | mac only. One image layer `hint` on top of the strip cells `hint_l0`, `hint_l2`, `hint_r0`, `hint_r1`, `hint_r2` (48×86 SVG, empty SVG when hidden). Knobs a tile can take: left 3, right 1–3; a strip the fork turns 180° is drawn into mirrored slots, so the hint for knob row r goes to cell 2 − r there (at 90° the weather hint is `hint_l0`). Owners per knob: l2 weather browse (not while chats browse), r0 keep-awake length edit + wi-fi pick, r1 chats browse, r2 timer edit + calendar list + chats browse; emoji is not an owner. Every daemon POST kicks a 60 Hz loop that runs while any hint shows: it asks the owners, shows / hides the chevron, and steps two springs (k = 360, c = 17): angle (each detent +650 °/s in the turn direction, clamp ±70°, clockwise = clockwise) and scale (target 1.4 while the knob is pressed, 1 after; a click < 400 ms without a turn kicks +9/s). Opacity 30 %, 100 % while the knob is held or for 0.7 s after a turn / press (eases in 0.25, out 0.08 per frame). The chevron sits 5 px from the outer edge; for the top / bottom knob its center is 10 px from the top / bottom of the cell (middle knob: centered), in the knob's own frame, white with a black halo, pointing at the knob; turned 180° when the fork turns that strip (`stripFlipped`, orientation read when it appears). Turns and presses of a hidden hint are ignored |
| palette | `palette.go`, top of `gen_pages.py` | white values, gray `#8E8E93` secondary, green `#30D158`, amber `#FFD60A`, red `#FF453A` (D1); content inset 6 % (D2) |

### Holds and haptics

- **Companion hold**: `button(..., held=...)` writes an action set named `"400"` and lists it in `runWhileHeld`. It fires once
  at 400 ms while still pressed; `up` then runs only for a shorter press. The fork plays a second haptic at 400 ms only for
  controls in `companion/hold-controls.txt` (generated: mac `button-0-3,button-1,button-1-2,button-1-5` = wi-fi tile, кнопка 1,
  play tile, agents tile; windows `button-1-5`).
- **Daemon-timed holds** (no second haptic): track knob like (400 ms, `trackknob.go`), push-to-talk (300 ms, `call.go`),
  keep-awake sleep (1 s / 3 s, `power.go`), dictation (400 ms), clock cancel (400 ms, `timers.go`). The daemon gets `down` and
  `up` and measures the time itself.
- Every press of a knob, tile or button gives a short haptic from the fork; strip slides never do. The daemon cannot drive
  the motor.

## 2. Readiness and how to check it on Windows

A port step is done when all of these hold:

1. `make test` is green on the Mac: `tools/check_contract.py` (every page URL hits a served route, every `curl` has `-m 1`,
   no mac-only port on the windows profile, variables ↔ daemon), Go tests, Swift checks. The mac `companion/` output is
   byte-identical (A6).
2. `GOWORK=off GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet ./...` and `go build -ldflags=-H=windowsgui .` succeed in
   `tools/zored_stream` (today they fail on the symbols in [§6](#6-what-a-windows-build-must-provide)).
3. On the PC, with Companion running and the windows page imported, for every route the windows page uses
   ([§5](#5-the-windows-page-as-generated-today)):
   ```powershell
   curl.exe -fsS -m 1 -X POST http://127.0.0.1:47113/volume/up      # 204, volume moved
   curl.exe -s http://127.0.0.1:8000/api/custom-variable/bar_value/value
   curl.exe -s -X POST http://127.0.0.1:8000/api/location/1/0/0/press  # Companion runs the knob's down set
   ```
   Inputs: with debug logging, Companion logs `Button <page>/<row>/<col> pressed`. State: the variable reads back. Looks: web
   preview `http://127.0.0.1:8000/buttons/1`; strips, LEDs and haptics only on the device (ask the user).
4. Stop Companion: the daemon keeps running and logs `set <var>: …` errors; start it: within 10 s every variable is back (A4).
5. Kill the daemon: every button still runs its exec, which fails fast (`-m 1`) and does nothing else; nothing hangs.

## 3. Controls

Each entry: **input → daemon route → what the daemon does → variables → what Companion draws**, then the Windows status.
"Win:" says what the windows profile does today and what the Windows daemon must do.

### Left side

#### крутилка слева, ряд 1 — volume (`0/0`, `:47113`)

- turn → `/volume/up` | `/volume/down`: if the emoji picker is open, it scrolls the left emoji column instead (`em.turn`).
  Else `volume += accel.step(dir)` (1–4 %), clamped 0–100; any step > 0 while muted unmutes; writes the system volume;
  `bar.show("volume", level)` (level = 0 while muted).
- press → `/volume/mute`: emoji picker open → types the left top emoji; window switcher open → a press starts (release `/volume/release` = click, hold = close window / quit app). Else toggles mute, shows the bar.
- external changes (keyboard, OS slider) arrive by OS notification, not a poll, and also show the bar.
- variables: `volume` (`"0"…"100"` or `MUTE`, not drawn), `bar_*`.
- macOS: CoreAudio `VirtualMainVolume` + mute on the default output, property listeners incl. default-device changes.
- Win: same routes. Core Audio `IAudioEndpointVolume` (`SetMasterVolumeLevelScalar`, `SetMute`) on the default render
  endpoint; notifications via `IAudioEndpointVolumeCallback` + `IMMNotificationClient` (default device change) *(assumed)*.

#### крутилка слева, ряд 2 — seek / track (`1/0`, `:47119`)

State machine `trackKnob` (`trackknob.go`):

| Input | Route | Effect |
|---|---|---|
| turn, not pressed | `/seek/forward`, `/seek/back` | seek (below) |
| press | `/track/down` | `pressed = true, used = false`, start a 400 ms timer |
| turn while pressed | same seek routes | `used = true`, previous / next track: media key `prev`/`next` + skip arrow animation |
| 400 ms passed, pressed, no turn | timer | `used = true`, Yandex Music like (`/like` logic of `:47115`); `409` unless `np_app` is Yandex Music, error only logged |
| release | `/track/up` | if `!used` → media key `play` (play/pause toggle); else nothing |

Seek (`seek.go`): needs a Now Playing duration > 0 (else `409`). If the last seek turn was > 1 s ago, the target starts at the
extrapolated position. `target += ±{2,4,8,15}[accel]` s, clamped to 0 … duration − 1 s (F18). `bar.show("seek", target/duration×100)`.
The seek command goes to the player at most every 150 ms; the last target always goes out (a trailing timer). After a seek
`np_progress` is re-published from the new position.

Skip arrow (`skip.go`): `np_skip` = `1`/`-1` while flying, `0` after; `np_skip_x` eases (smoothstep) from −90 to 110 (%) in
800 ms, mirrored for prev; 60 Hz while flying. Drawn as ⏩/⏪ on the bottom left-strip cell.

- Win: keep; hold-like dropped (no Yandex Music path, so the timer should do nothing, not `409`-spam). Seek via the Now
  Playing child's `seek` command (GSMTC `TryChangePlaybackPositionAsync`).

#### крутилка слева, ряд 3 — chats / weather (`2/0`, `:47130`, mac-only feature)

- turn → `/knob/forward|back`: if chats are not browsing and the weather tile is browsing → weather walks the forecast. Else
  chats browse (first turn opens the browse on the first chat, then one chat per detent).
- press → `/knob/down`: weather browsing → back to now (and the matching `/knob/up` is swallowed); else = tap on the chats tile.
- release → `/knob/up`: chats tile release.
- hold: the chats `down` arms a 400 ms daemon timer while browsing; when it fires the picked chat is marked read (source
  call async, the chat hidden at once, browse moves on) and the matching `up` does nothing. Outside browse no timer: `up` = tap.
- Win: the windows page still has this knob and posts to `:47130`, but no chats/weather variables exist there. Either drop
  `2/0` from the windows profile, or serve `:47130 /knob/*` with `409` *(decision open, see §5)*.

#### left strip — track text, progress, bar (`0/1…2/1`)

Three cells draw one 270 px tall picture (`gen_pages.py` `volume_cell`), bottom to top:

1. `progress` box: colour `np_tint`, height `np_progress` % (each cell takes its third; bottom-up in the page, top-down while `strip_flip` = 1), opacity `100 − bar_alpha`.
2. `np` image: `np_svg` rotated 90°, opacity `(100 − bar_alpha) × np_fade / 100`.
3. `track` black box + `fill` box when the bar is visible: opacity `bar_alpha`, height `bar_value` % (mirrored the same way by `strip_flip`), colour gray
   `#8E8E93` when `bar_mode == "ctl"`, else white.
4. bottom cell only: skip arrows (`np_skip`, `np_skip_x`).
5. emoji column overlay while `emoji_on == 1` (mac only).

- slide → fork posts `/volume/drag?dy=`: emoji open → scrolls the left emoji column; else `volume += dy × 0.4` %, fractional
  remainder kept between posts.
- tap: nothing (a bare strip tap posts nothing, so it is not even user activity).
- Win: same; `emoji_strip` is already omitted by the windows profile.

### Tiles

#### тайл ряд 2, колонка 1 — play / cover (`1/2`, `:47113`)

- press → `/nowplaying/show`: the ⏸ glyph shows for 1.5 s (`glyph.show`).
- release → `/nowplaying/tap`: if a swipe happened during this touch → nothing (the flag is cleared); else media key `play`.
- hold 400 ms (Companion) → `:47115/like` (mac only; the windows profile has no hold).
- swipe (fork `/tile/swipe?id=button-1-2&dir=`): `l` = prev, `r` = next (media key + skip arrow), `u` = like, `d` = dislike
  (Yandex Music). Any other tile id → `404`. Unknown `dir` → `400`.
- variables: `np_art` (cover data URI, empty = none), `np_playing`, `np_glyph` (0–100), `np_fade`.
- draws: no cover → a big ▶ when paused / ⏸ when playing. Cover → the cover at `np_fade` opacity, plus the glyph for the
  current state at `np_glyph` opacity, size `80 + np_glyph/5` %.
- glyph logic (`cover.go`): paused → ▶ fades in and stays, but only after 600 ms of pause (a pause shorter than that, e.g. a
  track change, shows nothing); playing → ⏸ fades in on a state change or a press, holds 1.5 s, fades out. Same fade factors
  as the bar.
- Win: keep; swipes `l`/`r` only. Drop `u`/`d` from the action map; whether the fork then treats the `400` as "no swipe" is
  *(assumed, check the fork)* — safer to answer `404` for `u`/`d` on Windows.

#### тайл ряд 2, колонка 4 — agents waiting (`1/5`, `:47120`)

- release (short press) → `/agent/focus`: list sorted by state (input, done, working, read), inside a state oldest update first; focus
  the first entry after the cursor in that order (wraps; no cursor → the first). A `done` CLI session becomes `read` when
  focused. Empty list → open a new session.
- hold 400 ms → `/agent/new`: open a new session.
- draws `agents_svg` (pure Go SVG): one dot per session, up to 16 in a 4×4 grid, amber = `input`, white = `done`, gray =
  `working`, hollow gray = `read`; circles for top-level, squares for children; the cursor dot is bigger with a white ring
  that slides 150 ms; a count when > 16; a hollow gray circle when none. A Desktop chat whose title starts with an emoji shows that emoji instead of the dot: drawn over the status dot (amber / white / gray, hollow and dimmed when read); no emoji in the title (CLI, `misc:`) → the dot.
- Win: keep, Codex as the source (see the agents module). Focus = bring the Codex window / Windows Terminal to front.

#### тайл ряд 3, колонка 1 — mic in a call / unread chats (`2/2`)

- mac: posts `:47130/tile/22/down|up`; the `tile_22` slot routes to the call layer (`/mic/down|up` logic) while
  `call_mic ≠ none`, else to the chats layer. Draws the call layers when `tile_22 == call`, `unread_svg` when `unread`.
- Chats layer: `down` arms the 400 ms read hold (see крутилка слева, ряд 3), `up` = tap unless the hold fired. The hold is timed
  in the daemon, not in Companion, so the call layer keeps getting `up` for push-to-talk.
- Win: the windows profile draws only the call layers and posts straight to `:47114/mic/down|up` (no slot).
- Mic logic (`call.go`), the same on both: `down` remembers `was = isOn` and the press time, sets mic to `!was` at once;
  `up`: held ≥ 300 ms → set mic back to `was` (push-to-talk, F5); shorter → keep. No call / unknown state → `409`.
- draws by `call_mic`: `on` → white mic on red; `off` → white crossed mic on black; `pending` → amber `…`; `unknown` → amber `?`.
- Win plan: system capture-endpoint mute ([windows.md](windows.md)); publish `call_mic` = `on`/`off` from the real endpoint
  state (F4: never a local toggle), `unknown` when no capture device, never `pending` unless a change is awaited.

#### тайл ряд 3, колонка 4 — clock (+ timers on mac) (`2/5`)

- Win: clock only, no actions: `HH:MM` from `$(internal:time_h|time_m)` and `DD.MM` from `date_d|date_m`, built in the page by
  string lookup into base64 SVG pieces (`time_image`, `date_image`), no daemon.
- mac: timers (`:47129`, below).

#### mac-only tiles (dropped on Windows by the profile)

| Tile | Module | Behaviour summary |
|---|---|---|
| ряд 1, колонка 1 — headphones (`0/2`) | `:47123` | tap toggles Bluetooth headset ↔ speakers; auto-route to `Logi*` / USB mic on arrival; `headphones_svg` |
| ряд 1, колонка 2 — wi-fi (`0/3`) | `:47124` | hold 400 ms = wi-fi power (F21); tap opens a 5 s network pick on крутилка справа, ряд 1, join after a 1 s pause; `wifi_svg` |
| ряд 1, колонка 3 — layout (`0/4`) | `:47127` | tap = next keyboard layout; `input_svg` crossfades 0.3 s (12 frames) |
| ряд 1, колонка 4 — keep-awake (`0/5`) | `:47122` | down/up timed in the daemon: < 1 s toggles a 1 h session; 1–3 s then release = end + sleep; > 3 s nothing (F16); `power_svg`; each second: all displays asleep → `screen_off = 1` + white moon (every other MEDIA control blacked out), screen locked → white lock; system sleep → the fork's `close()` draws the moon |
| ряд 2, колонка 2 — emoji (`1/3`) | `:47128` | picker 4 s on both strips + knobs 1 (F22) |
| ряд 2, колонка 3 — window switcher (`1/4`) | `:47132` | idle: 2×2 icons of the 4 newest apps, dot = Dock badge, checked every 2 s; open: apps on the left strip (windowless last), window titles one per line on the right (ring white, others gray), the picked app's big icon on the tile, 4 s; raise only on a tap / knob click / tile tap; hold 0.4 s closes the ring window, +2 s quits the app; knobs 1 step the columns, chevrons on both ([windows-switch.md](windows-switch.md)) |
| ряд 3, колонка 2 — camera / weather (`2/3`) | `:47131` | `tile_23` slot: call → `/cam/down`; else weather 5 s browse |
| ряд 3, колонка 3 — day / hang-up (`2/4`) | `:47126` | `tile_24` slot: call → hang up; else calendar list 5 s (F19) |

Details per module in [§4](#4-daemon-modules). Portable candidates if wanted later: weather (pure HTTP), timers (pure Go except
the sound), keep-awake (`SetThreadExecutionState`), layout (`ActivateKeyboardLayout`), emoji typing (`SendInput`
`KEYEVENTF_UNICODE`). They are out of the current plan.

### Buttons (row 3)

On mac every row-3 action is wrapped by the choice gate (F1):
`curl -fs -m 1 -X POST ':47120/choice/<down|up>?b=N' >/dev/null || { <own action> ; }`; the `400` hold set is gated as `up`;
the LED colour becomes `choice_on == 1 ? choice_N : <own colour>`. The windows profile has no gate.

| Button | mac | Windows profile |
|---|---|---|
| кнопка 1 (`3/0`) | KVM: tap → `:47125/kvm/windows` (Wake-on-LAN + monitor input DP), hold → `/kvm/pip`; LED `kvm_led` pink, white 0.3 s | removed |
| кнопка 2 (`3/1`) | down/up → `:47114/peek/down|up`: down flips `peek`, up after ≥ 300 ms flips it back (like push-to-talk); no call → `409`. While `peek` `call` publishes `call_mic`/`call_cam` = `none` and `call_peek` = `1`, `inCall()` is false (slots and кнопка 5 route to the normal layer), `/mic` `/cam` → `409`; `call_mic` → `none` clears `peek`. LED blinks dim white while `call_mic ≠ none` or `call_peek == 1` | free |
| кнопка 3 (`3/2`) | mic: `:47114/mic/down|up` (logic as the mic tile); LED by `call_mic`: `on` red blinking dark 1 s in 4, `off` `#303030`, `pending` amber, `unknown` amber blinking 1 Hz, else black | **kept**: same routes, same LED |
| кнопка 4 (`3/3`) | camera: `:47114/cam/down` toggles the camera; LED by `call_cam` | removed |
| кнопка 5 (`3/4`) | `:47126/button/5/press`: in a call hang up, else the calendar tap; LED white in a call, else `cal_led` (green browsing, white) | removed |
| кнопка 6 (`3/5`) | free | free |
| кнопка 7 (`3/6`) | dictation `:47117/dictation/down|up` | removed |
| кнопка 8 (`3/7`) | free | free |

### Right side

#### крутилка справа, ряд 1 — brightness (`0/7`, `:47118`)

Each action is a sequential group: the exec writes the daemon's reply (controller brightness, plain integer) into
`ctl_brightness`, then Companion's `set_brightness` (surface `self`) applies `$(custom:ctl_brightness)`.

- turn → `/brightness/up|down`: first offered to keep-awake length edit, wi-fi pick, emoji right column (mac); if none takes
  it: knob not held → display brightness `+= accel` %, clamped 0–100, read back from the OS first (keyboard changes count),
  `bar.show("mac", value)`; knob held → controller brightness `+= accel`, clamped **10**–100 (F17), `bar.show("ctl", value)`.
- press → `/brightness/hold`: emoji open → types the right top emoji; else `controller = true`, shows the gray bar.
- release → `/brightness/release`: `controller = false`.
- hold 3 s without a turn, then every 2 s while held (fork, no daemon): orientation 90 → 0 → 270 → 180 → 90, file `~/.config/zored-stream/orientation`
  (missing = 90). 90: tiles drawn turned 270° clockwise, left strip cells turned 180° into mirrored slots, left strip touch
  `y → 270 − y`, swipe `u r d l` → `r d l u`; 180: tiles 180°, both strips as the left one at 90, swipes → `d l u r`;
  270: tiles 90°, the right strip as the left one at 90, swipes → `l u r d` ([companion.md](companion.md#loupedeck-surface-module-fork)).
- start: the controller brightness is read once from Companion's DB (`surfaces.config.brightness` of the `loupedeck:%` row)
  via `/usr/bin/sqlite3`; must be exactly one row, else exit.
- Win: keep. Display brightness: DDC/CI (`dxva2` `Get/SetMonitorBrightness`, slow: coalesce turns) or WMI for a laptop
  panel; DB at `%APPDATA%\companion\v5.1\db.sqlite` read with a pure-Go SQLite or Companion's API ([windows.md](windows.md)).
  Do not call keep-awake / wi-fi / emoji hooks: they are not wired on Windows.

#### крутилка справа, ряд 2 — agents walk / AI model (`1/7`, `:47117`)

- turn → `/ai/turn?d=±1`: chats browsing (mac) → next / previous unread message of the picked chat (clamped), the tile
  badge = messages after it, the sender's small avatar in groups, the strip shows it from the top; else if the knob is held
  (mac AI switch) → model + effort step; else → agents walk: adds the Claude chat
  in front as a hollow dot if missing, moves the cursor by `d` (wraps), opens that session at once, shows its title + folder
  on the right strip for 2 s.
- press → `/ai/down`; release → `/ai/up`: if the press was used for a model switch → nothing; else = the agents tile press
  (`/agent/focus`).
- mac model switch (F12): 9 stops Sonnet·Low … Fable·High, applied 400 ms after the knob rests, only on Claude Desktop's
  new-chat screen in front; elsewhere a 5 s gray preview. Variables `ai_model`, `ai_effort`, `ai_state`
  (`ok`/`pending`/`preview`/`idle`/`other`/`error`).
- Win: the windows page keeps this knob and posts to `:47117`. Recommended Windows behaviour with no model switch: serve
  `:47117` with `/ai/turn` → agents walk, `/ai/down` → no-op, `/ai/up` → agents focus, `/ai/slide` → no-op `204`. The agents
  strip note is rendered with CoreText today (`calImage`); Windows needs an SVG version or no note.

#### крутилка справа, ряд 3 — scroll (`2/7`, `:47121`)

Before the scroller, each input is offered to timers, then calendar, then chats (knob capture, mac only). Chats while
browsing: turn = scroll the shown message by half a line (eased at 60 Hz while moving), press = open the chat. Scroller (`scroll.go`):

| Input | Mode | Effect |
|---|---|---|
| click (down + up, no turn) | any | toggle wheel ↔ tab, write `wheel`/`tab` to the mode file (default `~/.local/share/zored-stream/scroll-mode`; missing = wheel, other content = exit) |
| turn | wheel | vertical wheel, `{1,2,4,8}[accel]` lines per detent, clockwise = content moves down |
| press + turn | wheel | horizontal wheel, clockwise = right |
| turn | tab | Ctrl down (once), Tab (+Shift counter-clockwise) per detent; Ctrl up 500 ms after the last detent |
| press + turn | tab | Cmd down (once), Tab (+Shift) per detent = app switcher; Cmd up on release (or a 5 s timer). For 3 s after that release (each detent extends it) a plain turn sends Cmd+` (+Shift) = next window of the same app, Cmd released 500 ms after the last detent |

Modifier safety (F6, S1): any held modifier is released on knob press, on release, on SIGINT/SIGTERM, and Ctrl + Cmd up are
posted once at start (covers a crash).

- macOS: `CGEventCreateScrollWheelEvent` (line units) and keyboard events to the HID tap.
- Win: `SendInput` `MOUSEEVENTF_WHEEL` / `MOUSEEVENTF_HWHEEL` (`WHEEL_DELTA` 120 per line; sign: positive = up / right, so
  clockwise vertical = negative); tab mode: Ctrl+Tab as is; the Cmd chords map to Alt+Tab (hold Alt, Tab per detent, Alt up on
  release) and, for "windows of the same app", there is no system chord — drop that 3 s phase or keep Alt+Tab *(decision open)*.
  Same release rules, plus `SetConsoleCtrlHandler`/process exit to cover SIGTERM equivalents.

#### right strip — AI limits (`0/6…2/6`, `:47116`)

One 48×258 SVG `ai_strip_svg` (pure Go `aiStripSVG`), each cell shows its third.

- default: white bar bottom-up = 5 h session %, text = time left in the session `h:mm`; rotated model + effort dots (1–5).
- while switching the model: text `day`, bar = weekly % scaled to the workday; after a switch, for 5 s (or while previewing,
  up to 15 s): days left in the week `3d`, bar = weekly %.
- stale (F10): last fetch failed, never happened, or older than 2 intervals → gray bar; a limit past its `resets_at` → empty.
  A red dot = the last fetch failed.
- `covered`: other modules can put a picture over the whole strip (calendar list, agents note, wi-fi pick, chats browse).
  The wi-fi note (`say`) is text only; the knob hint chevron is a separate layer (`knobhint.go`).
- slide → `/ai/slide?y=`: emoji right column, else the model by position (mac).
- Win: keep the strip and variables, Codex as the source (usage module below).

#### Choice mode (mac only)

While a question / permission card / next-step box is on screen in the frontmost Claude Desktop or iTerm2, кнопки 1–8 answer it
(F1–F3): кнопка 1 = skip / previous / Deny / Dismiss, 2–7 = options in order, 8 = option 7 or next / finish. A release is
consumed iff its press was (F2). Variables `choice_on`, `choice_1…8` (LED colours). Win: off; the windows profile is not gated
and the windows `custom-variables.json` still lists `choice_*` (harmless, unused).

## 4. Daemon modules

All addresses are `127.0.0.1:<port>`, set by a flag with that default in `main.go` (A2). Windows uses the same ports.

| Port | Flag | Module | Routes | Variables | Win |
|---|---|---|---|---|---|
| 47113 | `-volume` | volume, media keys, Now Playing routes, tile swipe | `/volume/{up,down,mute}`, `/volume/drag?dy=`, `/key/{name}`, `/nowplaying/{show,tap,next,prev}`, `/tile/swipe?id=&dir=` | `volume`, `bar_*`, `np_*` | on |
| 47114 | `-call` | VK WorkSpace call | `/mic/{down,up}`, `/cam/down`, `/call/end` | `call_mic`, `call_cam` | `/mic/*` only, system mic |
| 47115 | `-ymusic` | Yandex Music like | `/like`, `/dislike` | `ym_like` | off |
| 47116 | `-usage` | AI limits | `GET /usage`, `POST /usage/refresh` | `ai_strip_svg`, `ai_session`, `ai_session_reset`, `ai_day`, `ai_week`, `ai_week_reset`, `ai_level` | on, Codex source |
| 47117 | `-ai` | AI model switch, dictation, right-knob routing | `/ai/{down,up}`, `/ai/turn?d=`, `/ai/slide?y=`, `/dictation/{down,up}` | `ai_model`, `ai_effort`, `ai_state`, `dictation` | `/ai/*` → agents only |
| 47118 | `-brightness` | display + controller brightness | `/brightness/{up,down,hold,release}` (reply = integer) | `bar_*` (`ctl_brightness` is written by Companion) | on |
| 47119 | `-seek` | seek + track knob | `/seek/{forward,back}`, `/track/{down,up}` | `bar_*`, `np_progress`, `np_skip*` | on |
| 47120 | `-agents` | agents waiting, choice mode | `POST /agent/event`, `GET /agents`, `/agent/{focus,new}`, `GET /choice`, `/choice/{down,up}?b=` | `agents_svg`, `choice_on`, `choice_1…8` | on, Codex; choice off |
| 47121 | `-scroll` | scroll, Ctrl+Tab | `/scroll/{down,up,forward,back}` | — | on |
| 47122 | `-power` | keep-awake, sleep | `/power/{down,up}` | `power_svg`, `screen_off` | off |
| 47123 | `-headphones` | headphones | `/headphones/toggle` | `headphones_svg` | off |
| 47124 | `-wifi` | wi-fi + VPN | `/wifi/{touch,power}` | `wifi_svg` | off |
| 47125 | `-kvm` | monitor input switch | `/kvm/{windows,pip}` | `kvm_led` | off |
| 47126 | `-calendar` | calendar, reminders, `tile_24` | `/tile/24/press`, `/button/5/press` | `cal_card`, `cal_browse`, `cal_icon`, `cal_led`, `tile_24` | off |
| 47127 | `-input` | keyboard layout | `/input/next` | `input_svg` | off |
| 47128 | `-emoji` | emoji picker | `/emoji/tile`, `/emoji/{start,end}?s=&y=` | `emoji_on`, `emoji_l`, `emoji_r`, `emoji_tile` | off |
| 47129 | `-timers` | timers + alarms | `/timer/{down,up}` | `timer_svg`, `timer_ring` | off |
| 47130 | `-unread` | unread chats, `tile_22`, left knob 3 | `/tile/22/{down,up}`, `/knob/{down,up,forward,back}` | `unread_svg`, `tile_22` | off (but see §5) |
| 47132 | `-switch` | window switcher | `/switch/tile/{down,up}` | `switch_tile` (+ shared `emoji_on`, `emoji_l`, `emoji_r`) | off |
| 47131 | `-weather` | weather, `tile_23` | `/tile/23/press` | `weather_svg`, `tile_23` | off |

### Ported modules (Windows on)

#### volume + media keys (47113)

- State: `value` 0–100, `muted`, drag remainder, `accel`. `poll()` re-reads the OS on every audio notification.
- `/key/{name}`: `volup`, `voldown`, `mute`, `play`, `next`, `prev`; unknown → `404`; OS failure → `409`. macOS posts
  NX system-defined media key events (reaches whatever owns Now Playing, browsers too). Win: `SendInput` with
  `VK_MEDIA_PLAY_PAUSE`, `VK_MEDIA_NEXT_TRACK`, `VK_MEDIA_PREV_TRACK`, `VK_VOLUME_UP|DOWN|MUTE` *(assumed)*. The pages use only
  `play`/`next`/`prev` via the daemon; volume goes through the volume routes.
- `/nowplaying/next|prev` only start the skip arrow; no page posts them today.

#### Now Playing (no own port; variables via 47113, commands via 47119)

A child process streams the current media session; the daemon owns all derived state (`nowplaying.go`).

- **Child protocol** (keep it identical on Windows, a `powershell.exe` child projecting
  `GlobalSystemMediaTransportControlsSessionManager` is the plan):
  - stdout: one JSON object per line on every change (macOS child polls every 0.5 s):
    `{"title","artist","playing":bool,"app":"<player id>","artwork":"<base64 JPEG, ≤160 px, empty = none>","duration":s,"elapsed":s,"rate":1.0,"at":<unix seconds of elapsed>}`.
    Lines that fail to parse are logged and skipped.
  - stdin: `seek <seconds>`, `play`, `pause` (one per line). Stdin EOF → the child exits.
  - The daemon restarts the child 1 s after it exits.
- **Derived variables**: `np_title`, `np_artist`, `np_track` (`artist — title`), `np_playing` (`1`/`0`), `np_app`
  (macOS bundle id; Win: `SourceAppUserModelId`), `np_art` (`data:image/jpeg;base64,…` — the prefix is hard-coded, so the
  Windows child must send JPEG or the daemon must sniff the type), `np_svg` (track strip picture), `np_progress`, `np_tint`,
  `np_glyph`, `np_fade`.
- Empty title while one was shown → the old title is kept for 3 s (players blank it between tracks).
- Track change (artist + title key) → `np_fade` 100 → 0 over 150 ms, new values, 0 → 100 over 150 ms.
- `np_svg`: lowercase, gray artist `|` bold title in JetBrains Mono, each part cut to 23 runes, scaled to 85 % width; split
  into two lines when one line would be too wide; a heart (liked) or broken heart (disliked, 2 s) only for Yandex Music.
- `np_progress` = extrapolated `elapsed + (now − at) × rate` (while playing) / duration × 100, published on every update and
  every 3 s.
- `np_tint` (`tint.go`): the cover's accent: 12 hue buckets over pixels with saturation ≥ 0.25 weighted by saturation × value,
  the heaviest bucket's mean colour, saturation capped 0.55, value 0.28; gray `#383838` without cover; eases at 60 Hz (factor
  0.06). Decodes JPEG only.

#### mic (47114, Windows variant)

macOS finds the VK WorkSpace call service process, subscribes over gRPC to status and call notifications, mutes a newly
joined call that starts with mic or camera on, and changes media with `ChangeMyMedia` awaiting confirmation (5 s, `pending`
meanwhile, then resync); it also tails the old engine's `voip_*.log` ([vkcall.md](vkcall.md)). None of that exists on Windows.
Windows variant: same `/mic/down|up` semantics (§3), state from the default capture endpoint's mute (`IAudioEndpointVolume`
on `eCapture`), notifications instead of polling *(assumed)*, `call_mic` only. `inCall()` is `call_mic ≠ none`; nothing on
the windows page depends on it.

#### AI limits (47116)

- Fetch on start, then every `-usage-every` (15 min); errors back off ×2 up to 1 h, honouring `Retry-After`; waits in ≤ 1 min
  ticks against the wall clock (timers stop while asleep). `POST /usage/refresh` = fetch now unless the last fetch was < 1 min ago.
- `GET /usage` → `{"limits":[…], "fetched_at", "error", "next_at"}` from the cache, never fetches.
- Limits model: `kind` `session` (5 h window), `weekly_all`, `weekly_scoped` (per model, matched by `scope.model.display_name`
  prefix), `percent`, `resets_at`.
- Variables: `ai_session` (%), `ai_session_reset` (`15:04`), `ai_week` (%), `ai_week_reset` (`Mon 15:04`), `ai_day` =
  `5 × weekly% / workday index since the window start` (Mon–Fri, 1…5), `ai_level` from `ai_day`: < 50 `low`, > 80 `high`,
  > 90 `critical`, else `normal`.
- macOS source: `GET https://api.anthropic.com/api/oauth/usage` with the Claude Code keychain token, refreshed by the daemon
  (F11, [ai.md](ai.md)). Win source: the last `rate_limits` of the newest `%USERPROFILE%\.codex\sessions\YYYY\MM\DD\rollout-*.jsonl`
  (`primary` 300 min → `session`, `secondary` 10080 min → `weekly_all`), shape verified on the Mac ([windows.md](windows.md#codex-limits-verified-shape-on-the-mac)).
  "Fetch failed" then means "no rollout file / no `rate_limits` found"; staleness should follow the file's last event time.

#### brightness (47118), seek (47119), scroll (47121)

Behaviour in §3. State: brightness `controller` flag + controller level; seek `target`, last send time, pending flag;
scroller mode, pressed/turned, held modifier, release timer, app-windows deadline.

#### agents (47120)

- **Input**: `POST /agent/event`, body = Claude Code hook JSON
  `{"session_id","hook_event_name","cwd","tool_name","notification_type","tool_input"}`, headers `Agent-Pid`, `Agent-Host`
  (Desktop session id `local_…`), `Agent-Iterm` (iTerm2 session id), `Agent-App` (bundle id). Missing `session_id` or event →
  `400`. Hooks must never block the agent (S2: `curl -s -m 1 --noproxy "*" … || true`, async; `tools/agent_hooks.py`).
- **Event → state**: `PermissionRequest` → `input` (with the tool as permission); `PreToolUse` of `AskUserQuestion` → `input`
  with the questions; `Notification` `permission_prompt` / `elicitation_dialog` → `input`, `idle_prompt` → `done`;
  `Stop`, `StopFailure` → `done`; `PostToolUse`, `UserPromptSubmit` → `working`; `SessionStart`, `SessionEnd` → remove;
  anything else logged.
- **Expiry** every 10 s: `read` entries go after 1 min; others after the TTL (`-agents-ttl`, 4 h) or when the pid is gone.
  Claude Desktop chats are also discovered and marked read from Desktop's session files (F14), mac/Claude only.
- **Order** (F13): top-level before children, then newest activity first, then id.
- **Focus**: Desktop chat → `open claude://code/continue?session=<id>`; iTerm2 → AppleScript select; else `open -b <bundle>`;
  new session → `open claude://code/new`. The chat in front becomes the cursor (polled 1 s while Claude is "hot", 3 s cold).
- Win (Codex): feed `/agent/event` from Codex `notify` (`agent-turn-complete` → send as `Stop` = `done`) and, if Codex hooks
  give approvals, as `PermissionRequest` = `input`; map Codex's session id to `session_id`. Pid check: `OpenProcess` +
  `GetExitCodeProcess` instead of `syscall.Kill(pid, 0)`. Focus: `SetForegroundWindow` on the Windows Terminal / Codex app
  window, no per-tab focus. New session: start Codex in a new terminal *(assumed)*. The Desktop scan, the "chat in front" and
  the Claude hot/cold gate have no Codex equivalent: replace with no-ops.

### mac-only modules (Windows off)

| Port | Module | Data source on macOS | Key timings / thresholds | Windows analogue (not planned) |
|---|---|---|---|---|
| 47115 | Yandex Music like | AX tree of the app (C BFS, ≤ 20 000 nodes); rescan 2 s, backs off to 1 min; AX read 3 s cold | presses within 1.5 s ignored; heart only when the player bar title matches `np_title` (F15) | none: needs the app's UI automation |
| 47117 | model switch, dictation | Claude Desktop AX, keys to Claude's pid only (S7) | apply 400 ms after rest, preview 5 s; dictation hold 400 ms, music pause/resume rules (F8), queued worker (F9) | Codex `config.toml` model rewrite (idea) |
| 47120 | choice mode | AX of Claude Desktop / iTerm2 every 0.5 s | settle 300 ms, keep 3 polls | none |
| 47122 | keep-awake, sleep | Amphetamine via `osascript`, IOPM assertions every 2 s, re-read 30 s | tap < 1 s 1 h; sleep arm 1 s, cancel 3 s; length 15 min…12 h, edit window 5 s, apply 500 ms after the last turn | `SetThreadExecutionState`, `SetSuspendState` |
| 47123 | headphones | IOBluetooth + CoreAudio, poll 2 s | connect wait 10 s, error shown 3 s | Bluetooth + `IPolicyConfig` (undocumented) |
| 47124 | wi-fi + Viscosity | CoreWLAN, `ipconfig`, Control Center via `osascript`, poll 3 s, internet probe 15 s | pick 5 s, join after 1 s, link wait 15 s, VPN 1 s after, retry once after 20 s | `netsh wlan`, WLAN API |
| 47125 | KVM | Wake-on-LAN UDP + `m1ddc` | LED white 0.3 s | WOL is portable; DDC via `dxva2` |
| 47126 | calendar | EventKit, poll 1 min; CoreText PNG text | list 5 s, horizon 7 days, glow from 5 min before an event | Outlook / Graph |
| 47127 | layout | Text Input Sources, notification on the main run loop | crossfade 0.3 s | `ActivateKeyboardLayout`, `WM_INPUTLANGCHANGE` |
| 47128 | emoji | emoji data + CoreText glyphs, typing via `CGEventKeyboardSetUnicodeString` | open 4 s; friction 0.94, fling 2, tap 6 px | `SendInput` `KEYEVENTF_UNICODE`; rendering needs another font path |
| 47129 | timers | own JSON file `~/.config/zored-stream/timers.json` (bad file = exit), tick 250 ms, sound `afplay Glass.aiff` | edit 5 s; 1 min per detent, every 4 fast detents (≤ 200 ms apart) the step grows 2/5/10/15/30/60 min; alarm 5 min steps; ring 1 min, sound every 2 s; hold 400 ms cancels | pure Go + `PlaySound` |
| 47130 | unread chats | Telegram MTProto (keychain session), VK WorkSpace long-poll; CoreText PNG | browse 5 s, 4 chats on the tile, muted at 40 %, retry 30 s | Telegram part is portable Go; keychain → Credential Manager |
| 47132 | window switcher | CGWindowList owners, AX windows + raise, app icons | open 4 s; strip physics of emoji | `EnumWindows` + `SetForegroundWindow`, icons from `SHGetFileInfo` (assumed) |
| 47131 | weather | Open-Meteo HTTP, poll 20 min, retry 1 min doubling to 20 min | stale after 90 min, browse 5 s, 3 days | fully portable Go |

Also mac-only and outside the daemon: the on-screen panel `tools/screen` (Swift, [screen.md](screen.md)), the Hammerspoon overlay
reading `GET :47116/usage`.

## 5. The windows page as generated today

`python3 tools/gen_pages.py companion/windows windows` keeps these cells (everything else is a black blank):

| Cell | Control | Posts to |
|---|---|---|
| `0/0` | крутилка слева, ряд 1 | `:47113/volume/{mute,down,up}` |
| `1/0` | крутилка слева, ряд 2 | `:47119/track/{down,up}`, `/seek/{back,forward}` |
| `2/0` | крутилка слева, ряд 3 | `:47130/knob/{down,up,back,forward}` — **no Windows module, no variables**; drop it or serve `409` |
| `0/1…2/1` | left strip | (fork) `:47113/volume/drag` |
| `1/2` | тайл ряд 2, колонка 1 | `:47113/nowplaying/{show,tap}`, (fork) `/tile/swipe` |
| `1/5` | тайл ряд 2, колонка 4 | `:47120/agent/focus`, hold `/agent/new` |
| `2/2` | тайл ряд 3, колонка 1 | `:47114/mic/{down,up}` |
| `2/5` | тайл ряд 3, колонка 4 | none (clock) |
| `0/6…2/6` | right strip | (fork) `:47117/ai/slide` |
| `0/7` | крутилка справа, ряд 1 | `:47118/brightness/{hold,release,down,up}` |
| `1/7` | крутилка справа, ряд 2 | `:47117/ai/{down,up,turn?d=±1}` |
| `2/7` | крутилка справа, ряд 3 | `:47121/scroll/{down,up,back,forward}` |
| `3/2` | кнопка 3 | `:47114/mic/{down,up}` |

Ports the Windows daemon must serve: 47113, 47114 (`/mic/*`), 47116, 47117 (agents routing), 47118, 47119, 47120, 47121, plus
47130 if `2/0` stays. `tools/check_contract.py` checks routes only against the mac `main.go`, and its `MAC_ONLY_PORTS` does
not list 47127, 47129, 47130, 47131, so the `2/0` gap is not caught today.

## 6. What a Windows build must provide

Cross-compiling the current package (`GOOS=windows CGO_ENABLED=0 go build -gcflags=-e .`) fails on exactly these undefined
symbols. Each line: the shared file that needs it → what to do.

| Needed by | Symbols | For a Windows build |
|---|---|---|
| `volume.go` | `readAudio`, `writeVolume`, `writeMute` | `audio_windows.go` (Core Audio) |
| `main.go` | `watchAudio` | `audio_windows.go`: endpoint + default-device notifications, calls the callback |
| `main.go`, `trackKnob` wiring | `postMediaKey` | `mediakey_windows.go` (`SendInput`) |
| `brightness.go` | `readDisplayBrightness`, `writeDisplayBrightness` | `display_windows.go` (DDC/CI or WMI); also replace `/usr/bin/sqlite3` in `readControllerBrightness` |
| `scroll.go` | `postScroll`, `postModifier`, `postChord` | `scroll_windows.go` (`SendInput`; `modCmd` → Alt, `chordGrave` → decide) |
| `agents.go` | `syscall.Kill`, `frontPid`, `claudePid`, `itermCurrent`, `itermPid`, `claude` (hot/cold gate), `calText`/`calImage` (strip note) | split: pid check, focus and front detection per OS; an SVG strip note or none |
| `power.go` (holds `touched`, used by `main.go` `serve`) | `declareUserActivity`, Amphetamine and sleep symbols | move `touched`/`activity` to a shared file; Windows `declareUserActivity`; keep-awake stays off |
| `main.go` | `newAISwitch`, `aiPollEvery`, `newDictation`, `dictatePollEvery`, `readChoiceCard`, `newYmusic`, `inputNext`, `inputWatch`, `claude` | module wiring per OS (`modules_darwin.go` / `modules_windows.go`, [windows.md](windows.md#daemon)); the Windows wiring registers only the ported modules, no stubs |
| `call.go` | `unix.SysctlKinfoProcSlice`, `unix.SysctlRaw`, `unix.ByteSliceToString` | mac-only; `mic_windows.go` provides the `/mic/*` handlers and `call_mic` |
| `nowplaying.go` | `/usr/bin/perl` child path | Windows child command (`powershell.exe -File …`), same line protocol |
| `calendar.go`, `chats.go`, `emoji.go`, `headphones.go`, `input.go`, `unread.go`, `voip.go`, `wifi.go` | CoreText renderers, EventKit, IOBluetooth, TIS, AX, CoreWLAN | mac-only, excluded from the Windows build |

`main.go` itself also hard-codes mac paths in flag defaults (`~/.local/share/…`, `~/Library/…`, `/opt/homebrew/bin/m1ddc`) and
calls `inputWatch` last, which on macOS runs the main-thread run loop; the Windows `main` must block on something else.

## Docs that disagree with the code

Found while writing this file (2026-11-10). The code is right in every case.

| Doc | Says | Code |
|---|---|---|
| [windows.md](windows.md) per-control table | left knob 2 = prev/next, press = play, hold = like; left knob 3 = seek | knob 2 (`1/0`) = seek / press+turn skip / click play / hold like; knob 3 (`2/0`) = chats / weather |
| windows.md, [controls.md](controls.md#windows) | mic = тайл ряд 3, колонка 1 + **кнопка 2**; camera / hang-up on кнопка 4 | mic button is **кнопка 3** (`3/2`), camera кнопка 4 (`3/3`), hang-up кнопка 5 (`3/4`); the windows page keeps кнопка 3 |
| windows.md, controls.md | freed on Windows: кнопки 3, 4, 7 (controls: 1, 3, 4, 7) | the profile removes кнопки 1, 4, 5, 7; кнопка 3 stays (mic) |
| windows.md daemon table | stops at 47122; "Now Playing + seek" on 47119 | 19 modules up to 47131; Now Playing has no port, its routes are on 47113, seek + track on 47119 |
| windows.md | shared files: `agents.go`, `brightness.go` … | `agents.go` needs mac symbols (§6), `brightness.go` shells out to `/usr/bin/sqlite3`, `touched` lives in mac-only `power.go` |
| windows.md volume row | "poll as now" | macOS uses CoreAudio listeners, no poll (P2); Windows should use endpoint notifications |
| windows.md pages | the windows profile drops only call/ym/keep-awake/dictation controls | it also drops headphones, wi-fi, layout, emoji, camera/weather, day/hang-up, кнопки 1, 4, 5, 7, timers; but keeps left knob 3 posting to `:47130` |
| [companion.md](companion.md#loupedeck-surface-module-fork) | `HAPTIC_HOLD_CONTROLS` is `encoder-1-0` (the track knob) | generated `companion/hold-controls.txt` = `button-0-3,button-1,button-1-2,button-1-5`; the track knob hold is daemon-timed without a second haptic |
| [macos.md](macos.md#zored_stream--seek) | seek is крутилка слева, ряд 3 | ряд 2 (`1/0`) |
| macos.md `np_svg` | artist and title cut to 30 characters with `…` | 23 runes (`stripMaxRunes`), no ellipsis |
| [unread.md](unread.md) body | Dock badges via AX on 47130 | the note at its top is right: `unread.go` is not wired (`newUnread` is never called); 47130 serves the chats module |
| [invariants.md](invariants.md) A6 | the Windows profile drops `call_cam`, …, `emoji_*` | it also drops `tile_22`, `tile_23`, `weather_svg`, `unread_svg` |
