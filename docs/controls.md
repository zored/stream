# Controls

What every control on the MEDIA page (page 1) does, from the user's side. Technical details live in the linked docs.
Source of truth: `tools/gen_pages.py` + `tools/zored_stream/`. **Update this file in the same commit when a control changes.**
Unconfirmed points are marked *(assumed)*; everything else was read in the code and verified in the feature's doc.

## Layout

```
          left                         tiles                           right
        knob strip   колонка 1   колонка 2   колонка 3   колонка 4   strip knob
ряд 1   VOL   ┐      en/ru       ·           ·           awake       ┐     SUN
ряд 2   TRACK ├trk   play/cover  emoji       ·           agents      ├ai   AI
ряд 3   SEEK  ┘      mic         camera      day / call  clock       ┘     SCROLL

        кнопка 1  2      3       4        5   6        7          8
        ·         mic    camera  hang up  ·   agents   dictation  ·
```

`·` = free. Grid coords for code: `row/col`, knobs `r/0` and `r/7`, strips `r/1` and `r/6`, tiles `r/2–5`, buttons `3/0–7`
([companion.md](companion.md)).

Everywhere: a short haptic tap on every press of a knob, tile or button, no haptics while sliding along a strip.
Hold = 400 ms: the action and a second haptic fire at 400 ms while still pressed; the release after it does nothing. The haptic list (`companion/hold-controls.txt`) is generated from every control with a hold. Exceptions timed on release: keep-awake sleep, push-to-talk, dictation.

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

### тайл ряд 1, колонка 1 — keyboard layout
- shows the current layout as a small flag: 🇬🇧 / 🇷🇺 (other languages: the code, white), crossfades in 0.3 s on a switch; follows ⌃Space and any other switch
- touch: next enabled layout (two enabled now, so EN ↔ RU)


### тайл ряд 1, колонка 2 — wi-fi

- shows: the current network name under the fan (white, cut to 10 chars); while joining, hollow fan + the target name in gray (up to 15 s); a phone instead of the fan on an iPhone hotspot (router in 172.20.10.0/28); gray hollow fan = wi-fi off; white hollow = on, not joined; white filled = joined; plus a yellow dot = no internet (captive.apple.com probe every 15 s); red for 3 s = failed; a dot top-right = Viscosity VPN: white connected, yellow connecting, red all down, none when Viscosity is not running
- hold 400 ms: wi-fi on / off
- touch: for 5 s крутилка справа, ряд 1 picks a network; the right strip shows the current network (or `offline`) instead of the model, yellow while the list loads, with a small `›` toward the knob until the first turn. Each turn extends the 5 s. List: what the Wi-Fi details of Control Center show (iPhone hotspots, then known networks in range); Control Center flashes and the list loads in ~3 s; meanwhile the strip is yellow and the knob already turns through the previous list. If the pick is gone from the fresh list, the first network is picked; a pause on a yellow pick joins after the load. Names keep their case (`zored_5G` ≠ `zored_5g`) A 1 s pause switches: drop the current Wi-Fi, disconnect Viscosity, join the pick, wait for its link, 1 s later connect `Barbos_SE` (flag `-wifi-vpn`; one retry if not connected in 20 s). The join itself: the daemon clicks it in the Wi-Fi details of Control Center (it flashes for ~1.5 s), macOS fills the password itself

### тайл ряд 1, колонка 3 — headphones

- shows the current output: speaker = wired / built-in, headphones = any Bluetooth output; yellow = connecting; red for 3 s = switch failed
- touch: toggle. To headphones: connect over Bluetooth, output and mic go to them (the mic always, so music drops to headset quality).
  Back: output to Logi, else MacBook; mic to a USB camera, else MacBook; then Bluetooth disconnects
- auto: when Logi or the USB camera appear (KVM switched to this Mac) and the output isn't headphones, output goes to Logi, mic to the camera; a manual choice afterwards stays
- no hold

[macos.md](macos.md)

### тайл ряд 1, колонка 4 — keep awake / sleep

- shows: hollow white sun = off; filled sun + time left `h:mm` below = an Amphetamine session, the display won't sleep; white moon = release now to sleep
- touch shorter than 1 s: keep-awake on for 1 h / off
- right after turning on, крутилка справа, ряд 1 sets the length for 5 s (time turns white): 15 min per detent, 15 min – 12 h;
  every turn extends the 5 s, then the knob is brightness again
- hold 1–3 s, then release: keep-awake off, the Mac sleeps; past 3 s the moon goes away and release does nothing
- no second haptic: the moon is the cue

[macos.md](macos.md)

### тайл ряд 2, колонка 1 — play

- shows: the track cover; ▶ stays while paused; ⏸ appears on a state change or a touch, holds 1.5 s, fades. No cover → a big ▶/⏸
- touch: shows the glyph; release: play / pause
- hold: Yandex Music like, as on крутилка слева, ряд 2
- swipe left / right (≥ 40 px within 400 ms): previous / next track; up / down: Yandex Music like / dislike.
  A swipe skips the play / pause on release. Dislike works only where the player bar shows it (not on radio or album contexts)

[macos.md](macos.md), [ymusic.md](ymusic.md)

### тайл ряд 2, колонка 2 — emoji picker

- idle: a random emoji picked at daemon start, smaller than while picking
- touch: opens the picker for 4 s, every action below extends it; shows the highlighted emoji of the last used strip
- touch while open: types that emoji into the focused field
- while open: the top item of each strip sits in a gray ring;
  left = recent emoji, then all of them; right = categories (🕘 recent, 😀, 👋, 🐻, 🍔, ⚽, 🚗, 💡, 🔣, 🏁), kept in sync with the left column: picking one jumps the left column to its start
- strip swipe: scrolls its column, flicks keep coasting; tap on an item lower down brings it to the top; tap on the left top emoji types it
- крутилка слева, ряд 1 / справа, ряд 1: turn scrolls its column (faster turns step more), press types the left top emoji; volume and brightness are off while the picker is open

[emoji.md](emoji.md)

### тайл ряд 2, колонка 3 — free

### тайл ряд 2, колонка 4 — agents waiting

- shows: one dot per Claude Code session: amber = needs input, white = done and unread (a Desktop chat stays white while its sidebar shows the blue dot), gray = working; white ring = last focused;
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

### тайл ряд 3, колонка 3 — day / hang up

- one tile, two layers, top active wins: a VK WorkSpace call > the day; the daemon picks the layer (`tile_24`) and routes the tap to it
- call: hang-up icon, tap ends the call. Same as кнопка 4. [vkcall.md](vkcall.md)
- day: all white, left-aligned: the date `3 ноя`; below `+40m` / `+5h` / `+1d` until the next event or reminder within 12 h (for a running event: `25m` until its end) and its title, lowercase, wrapped (reminders get `○`)
- titles keep emoji (drawn by CoreText into a PNG, the SVG renderer had none) and wrap by character; a recurring event is listed once, at its next occurrence
- reminders: open ones due up to +7 days `○`, ones completed today `●`
- touch: 5 s list, one item at a time; крутилка справа, ряд 3 scrolls it. The right strip (over model + effort) shows only the title white and the notes gray. The tile shows the item: its date `5 ноя`, below the weekday and time `ср 12:00` (amber for an event in ≤ 5 min or an overdue reminder), below `○` / `●` for a reminder
- in the list a touch (or the knob press) toggles the reminder (list stays open) or opens the event in Calendar

### тайл ряд 3, колонка 4 — clock + timers

- shows: `HH:MM` white, vertically centered when no timer; below it the nearest timer gray, to the minute: `12m`, `1h05`, an alarm `07:30`; more pending = ` +2`
- tap: set a timer for 5 s, крутилка справа, ряд 3 sets it (1 / 5 / 15 / 60 min per detent by speed), the value white; a second tap, the knob press or 5 s idle starts it, 0 = nothing
- turn left from 0: an alarm instead, time of day from the next 5 min, 5 min per detent
- hold ≥ 400 ms: cancel the nearest timer
- due: the tile blinks white and Glass plays every 2 s for 1 min; a tap silences it
- timers survive a daemon restart (`~/.config/zored-stream/timers.json`), a missed one rings on start

## Buttons

LED color = state. In choice mode (below) кнопки 1–8 answer Claude instead.

| Button | Does | LED |
|---|---|---|
| кнопка 1 | KVM → Windows: press = Wake-on-LAN to the PC, then monitor input DisplayPort (`m1ddc set input 15`), the monitor moves USB and the controller with it; back only from Windows or the monitor menu. Hold = toggle PIP | pink, white 0.3 s on press |
| кнопка 2 | mic: tap toggles, hold = push-to-talk | green (dims 1 s every 4 s) = on, dim red = off, yellow = switching, blinking yellow = unknown, off = no call |
| кнопка 3 | camera toggle | as кнопка 2 |
| кнопка 4 | hang up | white while in a call |
| кнопка 5 | free | off |
| кнопка 6 | agents: press = focus next session, hold = new Code session | amber breathing = a session needs input, dim gray = only finished unread ones, white = none |
| кнопка 7 | dictation into Claude Desktop (below) | red = ready, blinking green = recording, off = no Claude window with a record button |
| кнопка 8 | free | off |

### кнопка 7 — dictation

| Gesture | Effect |
|---|---|
| hold, speak, release | push-to-talk: records while held, sends on release |
| tap | starts recording, keeps it on |
| tap while recording | stops and sends |
| hold while recording | stops, leaves the text in the composer, no send |

Music that plays when recording starts pauses and resumes when it ends (also when started or stopped with the mouse),
unless the track or player changed or playback was started meanwhile.

[agents.md](agents.md#dictation)

## Right strip — AI limits + model

- shows: a white bar bottom-up = Claude 5h session usage, time left in the session near the bottom (`1:42`);
  the model rotated (reads bottom-up), effort as gray dots (low 1 … max 5); a red dot in the corner = the last fetch failed
- while switching the model: the word `day` and the bar = workday-scaled weekly usage of the selected model;
  right after: days left in the week (`3d`) and the bar = weekly usage
- model white, gray while previewing; effort dots gray, amber = outside the knob's list, red = switch failed
- slide: picks the model + effort by position, bottom Sonnet·Low → top Fable·High *(on the device: assumed)*
- tap: nothing

- stale: gray bar = the numbers are old (fetch failed or >30 min); after a reset the bar and the time clear until the next fetch

Limits refresh every 15 min; the daemon renews the Claude Code login token itself. [ai.md](ai.md), [ai-switch.md](ai-switch.md)

## Right knobs

### крутилка справа, ряд 1 — brightness

- turn: Mac display brightness (built-in display); while the emoji picker is open, its categories; for 5 s after keep-awake turns on, its length instead (тайл ряд 1, колонка 4); for 5 s after a touch on wi-fi (тайл ряд 1, колонка 2), the network
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

- click (press and release without a turn): switch wheel ↔ tab mode; the mode survives restarts
- wheel mode, turn: mouse wheel under the cursor, clockwise = down; 1 / 2 / 4 / 8 lines per detent by speed
- tab mode, turn: Ctrl held, clockwise = Tab, counter-clockwise = Shift+Tab; Ctrl is released 500 ms after the last detent
- press + turn: wheel mode = horizontal wheel, clockwise = right; tab mode = Cmd held while pressed, clockwise = Cmd+Tab, counter-clockwise = Cmd+Shift+Tab (Contexts shows the window list), release picks; no mode switch
- while the clock list is open: turn scrolls the list, press = clock hold
- while a timer is being set (clock tap): turn sets it, press starts it; scroll is back after

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
| wi-fi, headphones, keep-awake, camera, hang-up tiles, кнопки 1, 3, 4, 7 | dropped, free |
| agents tile + кнопка 6 | Codex sessions |
| right strip, крутилка справа, ряд 2 | Codex limits; model switch later (the knob is still on the windows page, its daemon module isn't) |
| choice mode | off (no Claude Desktop) |
