# Controls

What every control on the MEDIA page (page 1) does, from the user's side. Technical details live in the linked docs.
Source of truth: `tools/gen_pages.py` + `tools/zored_stream/`. **Update this file in the same commit when a control changes.**
Unconfirmed points are marked *(assumed)*; everything else was read in the code and verified in the feature's doc.

## Layout

```
          left                         tiles                           right
        knob strip   колонка 1   колонка 2   колонка 3   колонка 4   strip knob
ряд 1   VOL   ┐      ·           ·           ·           awake       ┐     SUN
ряд 2   TRACK ├trk   play/cover  ·           ·           agents      ├ai   AI
ряд 3   SEEK  ┘      mic         camera      hang up     clock       ┘     SCROLL

        кнопка 1  2      3       4        5   6        7          8
        ·         mic    camera  hang up  ·   agents   dictation  ·
```

`·` = free. Grid coords for code: `row/col`, knobs `r/0` and `r/7`, strips `r/1` and `r/6`, tiles `r/2–5`, buttons `3/0–7`
([companion.md](companion.md)).

Everywhere: a short haptic tap on every press of a knob, tile or button; soft ticks every 10 px while sliding along a strip.
Hold = 400 ms; the second haptic at 400 ms fires only on крутилка слева, ряд 2 (`HAPTIC_HOLD_CONTROLS` in the `Makefile`).

## Left knobs

### крутилка слева, ряд 1 — volume

- turn: system volume, 1 → 4 % per detent, faster turns step more
- press: mute / unmute
- shows: the left strip turns into a white volume bar while it changes

[macos.md](macos.md)

### крутилка слева, ряд 2 — track

- turn: previous / next track; a ⏪/⏩ flies across the bottom of the left strip
- press (on release): play / pause
- hold: Yandex Music like / remove like (only when Yandex Music is the Now Playing app), second haptic at 400 ms

[macos.md](macos.md), [ymusic.md](ymusic.md)

### крутилка слева, ряд 3 — seek

- turn: seek in the current track, 2 / 4 / 8 / 15 s per detent by speed; live, the player follows while you turn
- press: nothing
- shows: a bright white position bar on the left strip, fades 1 s after the last turn

Players: Yandex Music verified; Chrome/YouTube, Music.app *(assumed)*. [macos.md](macos.md)

## Left strip — track

- default: current track rotated 90°, gray artist | bold title; ♥ when liked in Yandex Music;
  a dim progress bar behind it, bottom-up, tinted with the cover's color
- while volume / brightness / seek moves: a bar over the whole strip, fades out ~1 s after:
  white = volume or Mac brightness or seek, gray = the controller's own brightness
- swipe up / down: volume, 0.4 % per px
- tap: nothing

[macos.md](macos.md), [companion.md](companion.md) (module fork)

## Tiles

### тайлы ряд 1, колонки 1–3 — free

### тайл ряд 1, колонка 4 — keep awake / sleep

- shows: hollow gray circle = off, white dot = the display won't sleep, white moon = release now to sleep
- touch shorter than 1 s: keep-awake on / off
- hold 1–3 s, then release: keep-awake off, the Mac sleeps; past 3 s the moon goes away and release does nothing
- no second haptic: the moon is the cue

[macos.md](macos.md)

### тайл ряд 2, колонка 1 — play

- shows: the track cover; ▶ stays while paused; ⏸ appears on a state change or a touch, holds 1.5 s, fades. No cover → a big ▶/⏸
- touch: shows the glyph; release: play / pause
- hold: Yandex Music like, as on крутилка слева, ряд 2 (no second haptic here)

[macos.md](macos.md), [ymusic.md](ymusic.md)

### тайлы ряд 2, колонки 2–3 — free

### тайл ряд 2, колонка 4 — agents waiting

- shows: one dot per Claude Code session: amber = needs input, white = done, gray = working; white ring = last focused;
  a number above 8; a hollow gray circle when none
- press (on release): focus the next session (needs input → done → working, oldest first), next press moves on and wraps;
  none waiting → opens a new Code session
- hold: new Claude Desktop Code session

Same as кнопка 6. [agents.md](agents.md)

### тайл ряд 3, колонка 1 — microphone (VK WorkSpace call)

- tap: toggle the mic; hold ≥ 300 ms: push-to-talk (mic on while held; if it was on, muted while held)
- shows: dark green + mic = on, red + crossed = off, amber `…` = switching, amber `?` = unknown, black = no call

Same as кнопка 2. [vkcall.md](vkcall.md)

### тайл ряд 3, колонка 2 — camera (VK WorkSpace call)

- tap: toggle the camera (takes 1–3 s, amber `…` meanwhile)
- shows: same colors as the mic tile

Same as кнопка 3. [vkcall.md](vkcall.md)

### тайл ряд 3, колонка 3 — hang up

- tap: end the VK WorkSpace call
- shows: hang-up icon only while in a call

Same as кнопка 4. [vkcall.md](vkcall.md)

### тайл ряд 3, колонка 4 — clock

- shows: `HH:MM` white, `DD.MM` gray
- press: nothing

## Buttons

LED color = state. In choice mode (below) кнопки 1–8 answer Claude instead.

| Button | Does | LED |
|---|---|---|
| кнопка 1 | free | off |
| кнопка 2 | mic: tap toggles, hold = push-to-talk | green (dims 1 s every 4 s) = on, dim red = off, yellow = switching, blinking yellow = unknown, off = no call |
| кнопка 3 | camera toggle | as кнопка 2 |
| кнопка 4 | hang up | white while in a call |
| кнопка 5 | free | off |
| кнопка 6 | agents: press = focus next session, hold = new Code session | amber breathing = a session needs input, dim gray = only finished ones, off = none |
| кнопка 7 | dictation into Claude Desktop (below) | red = ready, blinking green = recording, off = no Claude window with a record button |
| кнопка 8 | free | off |

### кнопка 7 — dictation

| Gesture | Effect |
|---|---|
| hold, speak, release | push-to-talk: records while held, sends on release |
| tap | starts recording, keeps it on |
| tap while recording | stops and sends |
| hold while recording | stops, leaves the text in the composer, no send |

[agents.md](agents.md#dictation)

## Right strip — AI limits + model

- shows: a white bar bottom-up = Claude 5h session usage, time left in the session near the bottom (`1:42`);
  the model rotated (reads bottom-up), effort as gray dots (low 1 … max 5); a red dot in the corner = the last fetch failed
- while switching the model: the word `day` and the bar = workday-scaled weekly usage of the selected model;
  right after: days left in the week (`3d`) and the bar = weekly usage
- model color: white = applied, gray = preview, amber = outside the knob's list, red = switch failed
- slide: picks the model + effort by position, bottom Sonnet·Low → top Fable·High *(on the device: assumed)*
- tap: nothing

- stale: gray bar = the numbers are old (fetch failed or >30 min); after a reset the bar and the time clear until the next fetch

Limits refresh every 15 min; the daemon renews the Claude Code login token itself. [ai.md](ai.md), [ai-switch.md](ai-switch.md)

## Right knobs

### крутилка справа, ряд 1 — brightness

- turn: Mac display brightness (built-in display)
- press + turn: the controller's own brightness, never below 10 %
- shows: the left strip bar for 1 s, white = Mac, gray = controller

[macos.md](macos.md)

### крутилка справа, ряд 2 — AI model + effort

- turn: walks Sonnet·Low … Sonnet·High → Opus·Low … → Fable·High (9 stops, stops at the ends)
- press + turn: model only, effort stays
- press and release without a turn: re-applies the shown choice
- applies 400 ms after the knob rests, only on the Claude Desktop new-chat screen in front;
  elsewhere it previews for 5 s (gray label, the strip shows that model's limits), then shows the real model again

[ai-switch.md](ai-switch.md)

### крутилка справа, ряд 3 — scroll

- turn: mouse wheel under the cursor, clockwise = down; 1 / 2 / 4 / 8 lines per detent by speed
- press + turn: horizontal, clockwise = right

[macos.md](macos.md)

## Choice mode

While Claude Desktop or iTerm2 is in front and shows a question, кнопки 1–8 answer it; their own actions
(mic, camera, hang-up, agents, dictation) pause until the card is gone. With `zored_stream` down the buttons work as usual.

LEDs: red = the button does something now, green = picked option, white = the primary button, off = unused.

| | Question card, one question, single choice | Several questions or multiple choice | Permission card | Next-step box |
|---|---|---|---|---|
| кнопка 1 | skip | previous question; skip on the first | Deny | Dismiss (if the box has one) |
| кнопки 2–7 | option 1–6: pick and go on | single: pick and go on; multiple: toggle | allow buttons in order | reply buttons in order |
| кнопка 8 | option 7 | next question; finish on the last | — | — |

Priority when several are on screen: question card, permission card, next-step box.
CLI in iTerm2: question cards work the same; permission prompt кнопка 2 = yes verified, кнопка 1 = Esc *(assumed, not run)*.
[agents.md](agents.md#choice-mode)

## TEST page (page 2)

Every control logs `TEST <name> down|up|left|right` to the Companion log (`custom_log`): knobs `KL1–3`, `KR1–3`, strips `SL1–3`,
`SR1–3`, tiles `T1–T12`, buttons `B1–B8` in rainbow LEDs. For checking inputs after a module or firmware change.

## Windows

From the plan in [windows.md](windows.md); nothing runs on Windows yet.

| Control | Windows |
|---|---|
| volume, track, seek knobs, left strip, play tile, clock, brightness, scroll | same; no Yandex Music hold; brightness = monitor over DDC/CI |
| mic tile + кнопка 2 | system mic mute (proposal) |
| camera, hang-up tiles, кнопки 3, 4, 7 | dropped, free |
| agents tile + кнопка 6 | Codex sessions |
| right strip, крутилка справа, ряд 2 | Codex limits; model switch later |
| choice mode | off (no Claude Desktop) |
