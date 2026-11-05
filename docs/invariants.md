# Invariants

What must stay true whatever you change. Read before touching pages, the daemon, hooks or the docs.
Each line: the rule, why, where it is enforced, what breaks without it.
Enforced by: **CI** = `make check` / Go tests in `.github/workflows/ci.yml`, **test** = a Go test name, **code** = read in the code,
nothing automatic, **doc** = a written rule only. *(unconfirmed)* = taken from a doc, not found in the code.

`make test` runs everything automatic locally: `tools/check_contract.py` (pages ↔ daemon) and `go vet` + `go test` in `tools/zored_stream`.

## Architecture

| # | Rule | Why | Enforced | Breaks if violated |
|---|---|---|---|---|
| A1 | Three layers: Companion draws and runs actions, `zored_stream` owns state and every OS call (media keys included: `:47113/key/*`). Companion → daemon only `curl -fsS -m 1 -X POST http://127.0.0.1:<port>/…` in an `exec`; daemon → Companion only `POST :8000/api/custom-variable/<name>/value` | one way to wire anything; either side can restart alone | CI `check_contract.py` (every page URL hits a served route, every `curl` has `-m 1`) | dead buttons that fail silently (`curl -f`), Companion actions hanging on a slow daemon |
| A2 | Ports are fixed, one module each: 47113 volume + Now Playing, 47114 calls, 47115 Yandex Music, 47116 usage (`GET /usage`, Hammerspoon), 47117 AI model + dictation, 47118 brightness, 47119 seek, 47120 agents + choice, 47121 scroll, 47122 power, 47123 headphones, 47124 wi-fi, 47125 KVM, 47126 calendar, 47127 keyboard layout, 47128 emoji, 47129 timers, 47130 unread. Windows uses the same ports | pages, the module fork (`/volume/drag`, `/tile/swipe`, `/ai/slide`, `/emoji/start|end`) and the overlay hard-code them | CI `check_contract.py` (flag defaults in `main.go`, one serve block each, no duplicates) | the fork's strip swipes or the overlay talk to nothing |
| A3 | Custom variables are the contract: every `$(custom:x)` in a page is in that profile's `custom-variables.json`, every variable the daemon sets is in the mac one and vice versa (`ctl_brightness` is written by Companion's `exec`) | Companion refuses posts to a variable that doesn't exist *(unconfirmed)* | CI `check_contract.py` | a feature that works locally and is blank after a fresh `make deploy` |
| A4 | The daemon skips unchanged values and re-sends everything every 10 s (`resendEvery`) | a Companion restart heals itself; no posting storm | code `main.go` `vars.set`, `forget` | state lost until the next change after a Companion restart |
| A5 | Pages come only from `tools/gen_pages.py`; `companion/*` is build output, committed and deterministic (rng seeded by page name) | reviewable diffs, CI can compare | CI `make check` + `git diff --exit-code companion` | random ids churn every build, hand edits get lost |
| A6 | The mac profile output does not change when the Windows profile does; the Windows profile only drops cells and variables (`call_cam`, `ym_like`, `dictation`, `power_svg`, `headphones_svg`, `wifi_svg`, `kvm_led`, `cal_*`, `tile_24`, `timer_*`, `input_svg`, `emoji_*`) (both profiles post media keys to `:47113/key/*`) | the Mac setup must keep working while Windows is planned | CI diff on both `companion/` and `companion/windows/`; `check_contract.py` (no mac-only ports on Windows) | the working Mac page breaks for a feature that doesn't run anywhere yet |
| A7 | `make deploy` overwrites pages 1 (MEDIA) and 2 (TEST) in the one running Companion; one session at a time edits `gen_pages.py` or deploys | two deployers wipe each other's buttons | doc (`CLAUDE.md`) | lost work, half-wired pages |
| A8 | One process owns the serial port: Companion, the Loupedeck app or `tools/sniff.py` | the device speaks to one host process | doc | device "disconnected", nothing reacts |
| A9 | The daemon fails loudly: bad flags, an unreadable `scroll-mode` file, no audio device → exit; `KeepAlive` restarts it (`ThrottleInterval` 5 s); a failed action is `409` + a log line | no silent fallbacks; a gated button falls back on `409` (F1) | code `main.go`, plist | half-working state with no trace in the log |
| A10 | The Loupedeck module fork (`zored/companion-surface-loupedeck`, branch `zored_stream`) carries haptics, strip touch position and the strip → daemon posts; no upstream PR. Second haptic at 400 ms on every control with a 400 ms hold: `gen_pages.py` writes `companion/hold-controls.txt`, `Makefile` passes it as `HAPTIC_HOLD_CONTROLS` | upstream has none of it | `Makefile` `module`, doc | no haptics, strip swipes dead |
| A11 | The daemon binary is signed with the `zored-stream` certificate, identifier `com.zored.stream.zored_stream` | TCC keys the Accessibility grant on identifier + cert; ad-hoc signing loses it on every rebuild | `Makefile` `zored_stream-bin` | AX features (like, choice, model, dictation, keys) stop after a rebuild until re-granted |

## Feature contracts

| # | Rule | Why | Enforced | Breaks if violated |
|---|---|---|---|---|
| F1 | Choice mode wins over кнопки 1–8: while a question card, permission card or next-step box is on screen in the frontmost Claude Desktop or iTerm2, every row-3 button answers it and its own action (mic, camera, hang-up, agents, dictation) is skipped. Each action is `curl -f :47120/choice/{down,up}?b=N \|\| { own ; }`, the 400 ms hold set gated as `up`; daemon down or `409` → own action | one set of buttons, no mode switch by hand; a dead daemon never kills the buttons | CI `check_contract.py` (all row-3 actions gated with a fallback), test `TestChoiceGate` | a press both picks an option and mutes the mic, or buttons dead with the daemon down |
| F2 | A release belongs to its press: `up` is consumed by choice mode iff its `down` was, even if the card vanished in between | otherwise a pick's release toggles push-to-talk or dictation | test `TestChoiceGate` | stray mic/dictation toggles after answering |
| F3 | Choice mapping: кнопка 1 = skip / previous / Deny / Dismiss, кнопки 2–7 = options or buttons in order, кнопка 8 = option 7 or next/finish; priority question card > permission card > next-step box. LEDs: red = live, green = picked, white = primary, off = unused, only through `box0` colour | the LED follows the background layer only | tests `TestChoiceAction`, `TestChoiceLEDs`; priority: code `choice_darwin.go` | wrong answer sent to Claude |
| F4 | Call LEDs and tiles show only what VK WorkSpace reports (`call_mic`, `call_cam`: `on`/`off`/`pending`/`unknown`/`none`), never a local toggle; `409` without a call; a call that appears while the daemon runs is muted if joined with mic or camera on | the button must never lie about a live mic | code `call.go` | green LED with the mic actually off, or the reverse |
| F5 | Push-to-talk: mic hold ≥ 300 ms, timed in the daemon from down/up arrival | Companion's hold is 400 ms and fires once | code `call.go` | |
| F6 | Ctrl and Cmd are never left down: tab mode releases Ctrl 500 ms after the last detent and Cmd 5 s after it, both on knob press and release, on SIGINT/SIGTERM and once at start (covers a crash) | a stuck modifier turns every click and key into a shortcut | tests `TestScrollTabReleasesCtrl`, `TestScrollPressReleasesCtrl`, `TestScrollSignalReleasesCtrl`, `TestScrollTabPressHoldsCmd`, `TestScrollCmdReleasedByTimerAndSignal`; start: code `newScroller` | the Mac is unusable until Ctrl is pressed by hand |
| F7 | Scroll mode: a click toggles wheel ↔ tab and persists to `scroll-mode`; press + turn = horizontal in wheel mode, Cmd+Tab in tab mode (Cmd held until release, F6 rules), then 3 s of Cmd+` on plain turns, never toggles | | tests `TestScrollModes`, `TestScrollTabPressHoldsCmd`, `TestScrollAppWindowsAfterSwitch` | |
| F8 | Dictation pauses music only if it plays at the off→on edge; resumes only the same app + title, only if still paused and nothing played meanwhile (from 2 s after the pause); explicit `pause`/`play`, never the toggle | the user's music must not start by itself | test `TestDictationMusic` | music blasting during a call or after the user stopped it |
| F9 | Dictation handlers only queue events and return `204`; one worker applies them in order; an empty transcript is not sent | Companion's exec timeout would cut a send | code `dictate_darwin.go` | half-sent messages |
| F10 | AI limits: stale data is never shown as current: the bar is gray when the last fetch failed, never happened, or is older than 2 intervals; a limit past its `resets_at` is cleared | old numbers look like real headroom | tests `TestLimitsStale`, `TestStripValueAfterReset` | user runs out of limit while the strip says 30 % |
| F11 | Limits cost no model tokens: only `GET /api/oauth/usage`, every 15 min with backoff, never the `claude` CLI; the token refresh writes only `Claude Code-credentials`, keeping every other field (user-approved 2026-11-03) | limits must not eat limits; a lost refresh token logs the CLI out | code `usage.go`, `claude_token.go` (no `claude` exec anywhere) | spent plan, logged-out Claude Code |
| F12 | Model switch applies only on Claude Desktop's new-chat screen in front; elsewhere a 5 s gray preview; `⌘⇧E` is posted to Claude's pid only; a read-back mismatch shows `error` | running sessions are untouched; keys never land in another app | code `ai_darwin.go` | a shortcut fired into the wrong app, a running chat changed |
| F13 | Agents order: top-level chats, then sub-sessions (`spawnedFrom`), each newest activity first; a press focuses the next and wraps; the chat in front (Desktop newest `lastFocusedAt`, iTerm2 current session) becomes the cursor, so a press moves on from it; none → new Code session; hold → new Code session | | tests `TestAgentOrder`, `TestFrontDesktop`; code `agents.go` `focus`, `watchFront` | |
| F14 | Unread: a Desktop "done" chat = not archived, not a routine run, `lastTurnEdgeAt > lastFocusedAt` in Desktop's own session files; it is dropped only when the file says read, not by `SessionEnd`, a dead pid, the TTL, a daemon restart or passing it with the cursor. A CLI "done" drops when focused. A dropped "done" lingers 1 min as `read` (hollow circle, last) | Desktop reaps idle `claude` processes while the chat is unread | tests `TestDesktopScan`, `TestDesktopDone`; code `agents.go` `remove`, `expire`, `focus` | finished chats vanish from the tile before the user saw them |
| F15 | Yandex Music like only when `np_app` is Yandex Music; presses within 1.5 s ignored; the heart shows only when the player bar's title matches `np_title` | the app races on fast presses; a frozen AX tree moves an old like onto a new track | code `ymusic_darwin.go`, `nowplaying.go` | like toggled twice, a wrong heart |
| F16 | Keep-awake: touch < 1 s toggles a 1 h Amphetamine session; hold 1–3 s then release = end session + sleep; past 3 s release does nothing; for 5 s after turning on, крутилка справа, ряд 1 sets the length (15 min … 12 h); timing lives in the daemon | | code `power.go`, test `TestAwakeLeft` (format only); sleep itself *(unconfirmed: not triggered)* | an accidental sleep in the middle of work |
| F21 | Wi-fi: hold toggles power at 400 ms while pressed (Companion `held` set, `/wifi/power`); a touch opens a 5 s network pick on крутилка справа, ряд 1; join only after a 1 s pause on a network other than the current one | a passing turn never switches networks | code `wifi.go`, test `TestIpconfigLink` (parsing only) | dropped off the network by a stray turn |
| F22 | Emoji picker: one owner of volume / brightness knobs and both strips while open (4 s, extended by any action); closed, it takes nothing; typing goes through `CGEventKeyboardSetUnicodeString`, never the clipboard; strip frames only when an offset changes, ≤ 60 Hz | | code `emoji.go`, test `TestEmojiPicker` | volume jumps or text lands in the clipboard while picking |
| F17 | Controller brightness never below 10 % | a dark device looks dead | code `brightness.go` | |
| F18 | Seek target clamped to 0 … duration − 1 s; live seek at most every 150 ms, the last target always goes out | | code `seek.go` | skipping to the next track |
| F19 | Day tile (`2/4`, calendar layer): white, left-aligned: `3 ноя`, then delta + title of the next event/reminder ≤ 12 h (amber only in the list: ≤ 5 min / overdue); dark green background from 5 min before an event until its end, cleared for that event by any calendar action; a touch opens a 5 s list: the right strip (covers model + effort) shows title white + notes gray, the tile shows the item's date, weekday + time, `○`/`●` for a reminder, and a touch there acts; the list captures крутилка справа, ряд 3 (turn + press), scroll mode resumes after | user decision | code `gen_pages.py` `day_tile`, tests `TestCalKeepAndNext`, `TestCalBrowse`, `TestCalSoonBg` | the scroll knob stays stolen or the day vanishes |
| F23 | A shared tile has ordered layers in the daemon (`layers.go` `slot`); the top active one is published (`tile_24` = `call` / `cal`) and gets the tap, the last one is the default. Tile `2/4`: call (`call_mic` ≠ none) > calendar; tile `2/2` (`tile_22` = `call` / `unread`): call mic > unread chats, the layer's `up` gets the release | one tap route, a live call always wins the tile | code `layers.go`, `check_contract.py` (slot names are published vars), test `TestTileLayers` | a tap in a call opens the calendar, or the hang-up icon is gone |
| F24 | Clock timers: a tap captures крутилка справа, ряд 3 (turn + press, Ctrl released, S1) for 5 s of idle; a second tap, the knob press or the timeout starts the timer and scroll gets the knob back; a ringing tile takes only the silencing tap; timers persist in `~/.config/zored-stream/timers.json` (bad file = exit, A9) | the scroll knob never stays stolen; a timer set is not lost on restart | code `timers.go`, tests `TestTimerKnobCaptureAndReturn`, `TestTimerOrderPersistAndRing`, `TestAlarmFromZeroLeft` | scroll dead after a timer, or a timer silently gone |
| F20 | Every changed control is described in `docs/controls.md` in the same commit | the device has no labels | doc | nobody knows what a button does |

## Performance

| # | Rule | Why | Enforced | Breaks if violated |
|---|---|---|---|---|
| P1 | Animations run at 60 Hz at most (`animationHz`, `stripHz`), easing rescaled per frame | Companion's per-button debounce (10/20 ms) is the ceiling: above 60 Hz fewer frames come out | code `bar.go`, `usage.go`; measurements in [companion.md](companion.md) | choppier animations, more CPU |
| P2 | Idle CPU ~0: animation tickers run only while something moves; polls stay rare (audio is CoreAudio listeners, no poll; Yandex Music, timers, tile layers 0.25 s — Yandex Music AX read 3 s unless a track changed, a press or it is frontmost; choice 0.5 s, AI model, dictation 1 s (each 3 s unless Claude is "hot": 20 s after a front change, agent input/done, press); calls rescan, power (in-process IOPM check, `osascript` on change or 30 s), headphones, Yandex Music rescan 2 s; wi-fi 3 s (Viscosity `osascript` on utun change or 30 s); progress 3 s; agents 10 s, chat in front 1 s while Claude is hot (3 s cold; rescans only when a session directory changed); wi-fi internet probe 15 s; calendar 1 min; limits 15 min). Frontmost app is a notification, not a poll. Periodic publishers (calendar, timers, power) re-render only when their frame key changes (`frame.go`). A new poll needs a reason in its doc. Measured 2026-11-04, `top -l 7 -s 10`, Claude frontmost, music paused: 8.5 % before round 3 → 1.3 % after (dictation keeps its AX element and backs off on a miss, Yandex Music player-bar rescan backs off to 1 min). Left (pprof): `claudePid` lookups, agents session scan, choice, headphones device list 2 s, wi-fi | the daemon runs all day on a laptop | code constants; manual `top -pid $(pgrep zored_stream)` | battery drain, fan |
| P3 | A strip picture spanning 3 cells costs 3 renders and draws per frame; keep static parts in separate layers or cells | serial queue is shared by the whole device | doc [companion.md](companion.md) | other controls lag while a strip animates |

## Safety

| # | Rule | Why | Enforced | Breaks if violated |
|---|---|---|---|---|
| S1 | No stuck modifier keys: any feature that posts a key down posts its up on every path (timer, press, signal, start) and gets a test like F6 | see F6 | tests F6; review | unusable keyboard |
| S2 | Agent hooks never block or fail an agent: `async: true`, `curl -s -m 1 --noproxy "*" … \|\| true` | a stopped daemon must cost nothing; hooks inherit the corporate proxy | CI `check_contract.py` (reads `tools/agent_hooks.py`) | every Claude Code turn waits or errors when the daemon is down |
| S3 | Ask before writing the user's keychain or global configs (`~/.claude/settings.json`, `~/.codex/config.toml`, Desktop presets); `make agent-hooks` is run by the user. The only automatic write is F11's token refresh | those files belong to other tools | doc | the user's settings overwritten (happened once: `model: opusplan`) |
| S4 | Ask before sleeping the Mac or triggering the sleep path | it ends the session in front of the user | doc | |
| S5 | Firmware: flash or send update commands only after explicit confirmation in chat; vendor binaries stay out of git | bricking risk | doc | a dead device |
| S6 | Secrets stay out of the repo: the Companion REST key lives in `~/.config/zored-stream/companion-token`, the Claude token only in the keychain | public repo | doc; no tokens in the logs *(unconfirmed)* | leaked credentials |
| S7 | Keys go to a specific pid when the target is known (`CGEventPostToPid` for Claude); only scroll, Ctrl+Tab and emoji typing (into the focused field, by definition) use the HID tap, by design | a key aimed at Claude must not hit the app the user just switched to | code `ai_darwin.go`, `scroll_darwin.go` | shortcuts fired into the wrong app |

## Style (device)

| # | Rule | Enforced |
|---|---|---|
| D1 | Minimal: no labels or units, dots over words, lowercase text, white values, gray `#8E8E93` secondary, palette green `#30D158` / amber `#FFD60A` / red `#FF453A` (`palette.go`, top of `gen_pages.py`), color only for warnings or a transient state; offer the sparest variant first | doc, review |
| D2 | 6 % inset for content on every tile and strip picture | code (`x=v(6) … width=v(88)` in `gen_pages.py`, `inset` in `nowplaying.go`), review |
| D3 | Text in JetBrains Mono static weights; strip pictures are 48×258 (the drawable area), not 60×270 | doc [companion.md](companion.md) |
| D4 | Talk and docs name controls "крутилка слева, ряд 1", "тайл ряд 2, колонка 3", "кнопка 1"; grid `row/col` only in code and technical docs | doc |
| D5 | Icons come from Bootstrap Icons outline (MIT, pinned in `tools/zored_stream/icons/`, refreshed by `fetch.sh`): `iconAt` in the daemon, `bootstrap()` in `gen_pages.py`; a state is the icon color on a black tile, not a colored background | code, review |

## Process

| # | Rule | Enforced |
|---|---|---|
| R1 | New features go on the MEDIA page as buttons; a new page only once it runs out; take free controls from the map in `PLAN.md` and update it in the same commit | doc |
| R2 | Commit only your own paths: `git -C <absolute repo> commit -m main -- <paths>`; message is `main`; a git-guard hook enforces absolute `-C` and explicit paths | hook |
| R3 | Each new task gets a fresh child session; `PLAN.md` is kept by the master session | doc |
| R4 | Findings are marked verified (observed) or assumed (read) in docs | doc |
| R5 | Windows has only Codex, its own players, no VK WorkSpace; plan for it never changes the Mac setup (A6) | doc [windows.md](windows.md) |
| R6 | Before reporting done: `make test`, then the device checks from `CLAUDE.md` (inputs, state, looks) | doc |

## Manual checks

Not covered by tests; run them after touching the area.

- Choice mode on the device: a live question, permission card and next-step box in Claude Desktop; a question in the CLI in iTerm2 (F1–F3)
- Calls: mic tap / hold, camera, hang-up in a real VK WorkSpace call; LEDs follow mouse clicks in the client (F4, F5)
- Tab mode: Ctrl+Tab reaches the app, Ctrl is up after 500 ms (`kill -TERM` the daemon mid-turn, then type) (F6)
- Dictation: all four gestures; music paused and resumed with Yandex Music (F8, F9)
- Strips, haptics, LED colours, the 6 % inset: only on the device, ask the user (D1–D3)
- Idle CPU after a minute with nothing moving (P2)
- Keep-awake hold → sleep: never verified, ask first (F16, S4)
- Wi-fi: hold = power, pick + join through Control Center, Viscosity reconnect after the join (F21)
- Headphones: toggle both ways, auto-route when Logi / the USB camera appear
- KVM (кнопка 1): Wake-on-LAN + input switch to Windows, hold = PIP
- Keyboard layout tile follows ⌃Space
- Emoji picker: both strips, both knobs, typing into the focused field (F22)
- Day tile: list, reminder toggle, event opens in Calendar; a call takes the tile over (F19, F23)
- Timers: set, alarm, ring, silence, survive a restart (F24)
- Play tile swipes: prev / next, like / dislike; a swipe never toggles play

## Changing an invariant

1. Say in the commit (and to the user) which one you break, by number, and why.
2. Update this file in the same commit: the new rule, or remove it.
3. If it was enforced by CI or a test, change the check in the same commit; never delete a check to make CI green.
