# Controls

What every control on the MEDIA page (page 1) does, from the user's side. Technical details live in the linked docs.
Source of truth: `tools/gen_pages.py` + `tools/zored_stream/`. **Update this file in the same commit when a control changes.**
Unconfirmed points are marked *(assumed)*; everything else was read in the code and verified in the feature's doc.

## Layout

```
          left                         tiles                           right
        knob strip   колонка 1   колонка 2   колонка 3   колонка 4   strip knob
ряд 1   VOL   ┐      headphones  wi-fi       en/ru       awake       ┐     SUN
ряд 2   SEEK  ├trk   play/cover  emoji       ·           agents      ├ai   AI
ряд 3   CHATS ┘      mic/chats   cam/weather day / call  clock+timer ┘     SCROLL

        кнопка 1  2      3       4        5   6        7          8
        KVM       ·      ·       ·        cal  ·        dictation  ·
        in a call:       mic     camera   hang up
```

`·` = free. Grid coords for code: `row/col`, knobs `r/0` and `r/7`, strips `r/1` and `r/6`, tiles `r/2–5`, buttons `3/0–7`
([companion.md](companion.md)).

Everywhere: a short haptic tap on every press of a knob, tile or button, no haptics while sliding along a strip.
One temporary tile mode at a time: opening one (wi-fi pick, keep-awake length, emoji picker, chats browse, weather browse, day list, timer edit) closes any other open one, as its timeout would (a timer being set starts, a wi-fi pick not yet joined is dropped).
Knob hint: while a tile has taken a knob for a while (wi-fi, keep-awake, chats, weather, day, clock), a small white chevron sits at the outer edge of the strip next to that knob, pointing at it, 30 % opaque (100 % while turning or held);
every detent swings it in the turn direction and it springs back, it grows while the knob is held and pulses on a click; gone when the knob is given back. Not for the emoji picker (it owns both strips anyway) ([logic.md](logic.md#shared-primitives-all-portable-go))
Hold = 400 ms: the action and a second haptic fire at 400 ms while still pressed; the release after it does nothing. The haptic list (`companion/hold-controls.txt`) is generated from every control with a hold. Exceptions timed on release: keep-awake sleep, push-to-talk, dictation.

## Free controls

New features take free controls from this map; update it in the same commit.

MEDIA page, `row/col` ([grid](companion.md)):

| Used | Free |
|---|---|
| left knobs `0/0` volume, `1/0` seek (press + turn = tracks, click = play / pause, hold = like), `2/0` chats browse; left strip `0–2/1` track + bar | — |
| tiles `0/2` headphones, `0/3` wi-fi, `0/4` keyboard layout, `0/5` keep-awake, `1/2` play (hold = like, swipes), `1/3` emoji, `1/5` agents waiting, `2/2` mic / chats, `2/3` camera (call) / weather, `2/4` hang up / date + calendar, `2/5` clock + timers | tile `1/4` |
| buttons `3/0` KVM, `3/1` call layer (tap toggles, hold peeks), `3/2` mic / `3/3` camera / `3/4` hang up in a call, `3/4` calendar outside a call, `3/6` dictation; all of `3/0–3/7` in choice mode | buttons `3/5`, `3/7`; `3/2`, `3/3` outside a call |
| right strip `0–2/6` AI limits + model, right knobs `0/7` brightness (hold 3 s = device orientation), `1/7` AI model, `2/7` scroll / Ctrl+Tab | — |


## Left knobs

### крутилка слева, ряд 1 — volume

- turn: system volume, 1 → 4 % per detent, faster turns step more
- press: mute / unmute
- shows: the left strip turns into a white volume bar while it changes

[macos.md](macos.md)

### крутилка слева, ряд 2 — seek / track

- turn: seek in the current track, 2 / 4 / 8 / 15 s per detent by speed; live, the player follows while you turn; a bright white position bar on the left strip, fades 1 s after the last turn
- press + turn: previous / next track; a ⏪/⏩ flies across the bottom of the left strip
- click (press and release without a turn): play / pause
- hold ≥ 400 ms without a turn: Yandex Music like / remove like (only when Yandex Music is the Now Playing app); timed in the daemon, no second haptic

Players: Yandex Music verified; Chrome/YouTube, Music.app *(assumed)*. [macos.md](macos.md), [ymusic.md](ymusic.md)

### крутилка слева, ряд 3 — chats

- turn: browse the unread chats of тайл ряд 3, колонка 1 (the first turn opens the first chat, then each detent moves one)
- press: same as a tap on that tile: browse, or open the chat shown; hold 400 ms while browsing marks the chat read
- while the weather tile browses: turn walks the forecast, press returns to now

[unread.md](unread.md), [weather.md](weather.md)

## Left strip — track

- default: current track rotated 90°, gray artist | bold title; a white ♥ when liked in Yandex Music, a white broken heart for 2 s after a dislike;
  a dim progress bar behind it, tinted with the cover's color; all bars (this one and the volume / brightness / seek bar) grow bottom-up on the device in every orientation (pages mirror them while the left strip is turned 180°, i.e. at 90° and 180°)
- while volume / brightness / seek moves: a bar over the whole strip, fades out ~1 s after:
  white = volume or Mac brightness or seek, gray = the controller's own brightness
- swipe up / down: volume, 0.4 % per px
- tap: nothing

[macos.md](macos.md), [companion.md](companion.md) (module fork)

## Tiles

### тайл ряд 1, колонка 1 — headphones

- shows the current output: speaker = wired / built-in, headphones = any Bluetooth output; yellow = connecting; red for 3 s = switch failed
- touch: toggle. To headphones: connect over Bluetooth, output and mic go to them (the mic always, so music drops to headset quality).
  Back: output to Logi, else MacBook; mic to a USB camera, else MacBook; then Bluetooth disconnects
- auto: when Logi or the USB camera appear (KVM switched to this Mac) and the output isn't headphones, output goes to Logi, mic to the camera; a manual choice afterwards stays
- no hold

[macos.md](macos.md)

### тайл ряд 1, колонка 2 — wi-fi

- shows: the current network name under the fan (white, cut to 10 chars); while joining, the target name in gray (up to 15 s); a phone instead of the wi-fi icon on an iPhone hotspot (router in 172.20.10.0/28, filled when joined); gray crossed wi-fi = off; one white arc = on, not joined; full wi-fi = joined; plus a yellow dot = no internet (captive.apple.com probe every 15 s); red for 3 s = failed; an eye top-right = Viscosity VPN: white eye connected, yellow eye connecting, red crossed eye all down, none when Viscosity is not running
- hold 400 ms: wi-fi on / off
- touch: for 5 s крутилка справа, ряд 1 picks a network; the right strip shows the current network (or `offline`) instead of the model, yellow while the list loads. Each turn extends the 5 s. List: what the Wi-Fi details of Control Center show (iPhone hotspots, then known networks in range); Control Center flashes and the list loads in ~3 s; meanwhile the strip is yellow and the knob already turns through the previous list. If the pick is gone from the fresh list, the first network is picked; a pause on a yellow pick joins after the load. Names keep their case (`zored_5G` ≠ `zored_5g`) A 1 s pause switches: drop the current Wi-Fi, disconnect Viscosity, join the pick, wait for its link, 1 s later connect `Barbos_SE` (flag `-wifi-vpn`; one retry if not connected in 20 s). The join itself: the daemon clicks it in the Wi-Fi details of Control Center (it flashes for ~1.5 s), macOS fills the password itself

### тайл ряд 1, колонка 3 — keyboard layout
- shows the current layout as a small flag: 🇬🇧 / 🇷🇺 (other languages: the code, white), crossfades in 0.3 s on a switch; follows ⌃Space and any other switch
- touch: next enabled layout (two enabled now, so EN ↔ RU)


### тайл ряд 1, колонка 4 — keep awake / sleep

- shows: outline sun = off; filled sun + time left `h:mm` below = an Amphetamine session, the display won't sleep; white moon = release now to sleep; gray lock = the screen is locked; gray moon = the screens are off (the rest of the device goes black until they wake) or the Mac sleeps (the module fork draws it as Companion closes the surface)
- touch shorter than 1 s: keep-awake on for 1 h / off
- right after turning on, крутилка справа, ряд 1 sets the length for 5 s (time turns white): 15 min per detent, 15 min – 12 h;
  every turn extends the 5 s, then the knob is brightness again
- hold 1–3 s, then release: keep-awake off, the Mac sleeps; past 3 s the moon goes away and release does nothing
- no second haptic: the moon is the cue

[macos.md](macos.md)

### тайл ряд 2, колонка 1 — play

- shows: the track cover; ▶ stays while paused; ⏸ appears on a state change or a touch, holds 1.5 s, fades. No cover → a big ▶/⏸. On a track change the cover and the track strip fade out and in (150 ms each)
- touch: shows the glyph; release: play / pause
- hold: Yandex Music like
- swipe left / right (≥ 40 px within 400 ms): previous / next track; up / down: Yandex Music like / dislike.
  A swipe skips the play / pause on release. Dislike works only where the player bar shows it (not on radio or album contexts)

[macos.md](macos.md), [ymusic.md](ymusic.md)

### тайл ряд 2, колонка 2 — emoji picker

- idle: Bootstrap `emoji-smile` on top, the last 3 recent emoji below (after the picker was opened once)
- touch: opens the picker for 4 s, every action below extends it; shows the highlighted emoji of the last used strip
- touch while open: types that emoji into the focused field
- while open: the top item of each strip sits in a gray ring;
  left = recent emoji, then all of them; right = categories (🕘 recent, 😀, 👋, 🐻, 🍔, ⚽, 🚗, 💡, 🔣, 🏁), kept in sync with the left column: picking one jumps the left column to its start
- emoji stand upright in every device orientation (turned per glyph; the left strip is mirrored when the fork flips it; the orientation is read each time the picker opens)
- strip swipe: scrolls its column, flicks keep coasting; tap on an item lower down brings it to the top; tap on the left top emoji types it
- крутилка слева, ряд 1 / справа, ряд 1: turn scrolls its column (faster turns step more; direction inverted when the device is turned), press types the left top emoji; volume and brightness are off while the picker is open

[emoji.md](emoji.md)

### тайл ряд 2, колонка 3 — free

### тайл ряд 2, колонка 4 — agents waiting

- shows: one dot per Claude Code session: amber = needs input, white = done and unread (a Desktop chat stays white while its sidebar shows the blue dot), gray = working, up to 16 in a 4×4 grid; the active one (chat in front, else last focused) is bigger with a white ring,
  the ring slides over in 150 ms; a number above 16; a hollow gray circle when none
- order: needs input, done, working, read; inside a group oldest update first; all circles
- press (on release): focus the next session in that order, next press moves on and wraps;
  none waiting → opens a new Code session
- hold: new Claude Desktop Code session
- крутилка справа, ряд 2: turn moves the ring and wraps, the right strip shows the chat title white + folder gray for 2 s (as the calendar list), the chat opens at once (hollow dots of read chats help to step back); click = tile press
- a chat read in Desktop (or a CLI one focused) stays for 1 min at the end as a hollow gray circle

A knob turn with the chat in front not on the tile adds it as a hollow dot for 1 min. [agents.md](agents.md)

### тайл ряд 3, колонка 1 — microphone in a call, unread chats otherwise

- in a call: tap toggles the mic; hold ≥ 300 ms: push-to-talk (mic on while held; if it was on, muted while held)
- in a call shows: white mic on red = on, white crossed mic on black = off, amber `…` = switching, amber `?` = unknown
- no call: up to 4 unread chats (Telegram + VK WorkSpace, via their APIs, push), normal ones first, then muted (muted or archived in Telegram, muted or archived in VK WorkSpace), each group newest first; muted chats sit at 40% opacity with a gray count badge; one fills the tile, two sit on a diagonal (top-left, bottom-right), 3–4 in a 2×2 grid of squares on black: avatar, or without one the first words of the name in 2–3 lowercase lines on dark gray, unread count in a red badge top-right (`99+` cap), age of the last message in a dark badge bottom-right (`5m`, `3h`, `2d`); a gray `+` on the right when there are more; small white eye = nothing unread
- tap: browse for 5 s (any action below extends it), крутилка слева, ряд 3 scrolls the chats (same order) with a slide animation; the tile shows the chat's avatar at full opacity over the whole square, the right strip the first unread message in white, 3 lines; a second tap or the knob press opens that chat (Telegram `tg://`, VK WorkSpace profile link)
- while browsing: крутилка справа, ряд 2 walks the unread messages of that chat, oldest first, stops at the ends; the red badge counts the messages after the shown one (none on the last), a group chat shows the author's small avatar bottom-left; крутилка справа, ряд 3 scrolls the message smoothly, half a line per detent. Both knobs work as before once browse closes
- hold 400 ms while browsing (this tile or крутилка слева, ряд 3): marks the shown chat read in Telegram / VK WorkSpace, browse moves on to the next chat; the release does nothing; no second haptic. Outside browse a hold is a plain tap
- кнопка 3 does the same in a call, крутилка слева, ряд 3 outside a call
- [unread.md](unread.md) [vkcall.md](vkcall.md)

### тайл ряд 3, колонка 2 — camera (VK WorkSpace call) / weather

- in a call: tap toggles the camera (takes 1–3 s, amber `…` meanwhile), same colors as the mic tile; same as кнопка 4
- outside a call: weather icon, below the temperature white and the feels-like one gray, no separator; all gray when stale
- tap outside a call: 5 s browse (the date and part of day show only now), крутилка слева, ряд 3 walks the forecast (утро, день, вечер, ночь, next days),
  weekday, date and part of day (`чт 12 день`) white on top, only while browsing; a second tap or the knob press returns to now

[vkcall.md](vkcall.md), [weather.md](weather.md)

### тайл ряд 3, колонка 3 — day / hang up

- one tile, two layers, top active wins: a VK WorkSpace call > the day; the daemon picks the layer (`tile_24`) and routes the tap to it
- call: hang-up icon, tap ends the call. Same as кнопка 5 in a call. [vkcall.md](vkcall.md)
- day: all white, left-aligned: the date `3 ноя`; below the title of the nearest event or reminder, lowercase, wrapped (reminders get `○`), then gray `+40m` / `+5h` / `+1d` right after it, tomorrow or later if today is empty (for a running event: `25m` until its end)
- titles keep emoji (drawn by CoreText into a PNG, the SVG renderer had none) and wrap by character; a recurring event is listed once, at its next occurrence
- reminders: open ones due up to +7 days `○`, ones completed today `●`
- dark green background from 5 min before an event (not reminders) until it ends; any calendar action (touch, кнопка 5, knob in the list) clears it for that event
- touch: 5 s list starting on the item the tile shows, one item at a time; крутилка справа, ряд 3 scrolls it. The right strip (over model + effort) shows the title white with the notes right after it in gray, one run of text, HTML (`<br/>` = line break) and links, separator lines and call ID / access code lines cut out. The tile shows the item: its date `5 ноя`, below the weekday and time `ср 12:00` (amber for an event in ≤ 5 min or an overdue reminder), below `○` / `●` for a reminder
- in the list a touch (or the knob press) toggles the reminder (list stays open) or opens the event in Calendar
- кнопка 5 does the same tap outside a call, hangs up in a call

### тайл ряд 3, колонка 4 — clock + timers

- shows: `HH:MM` white, vertically centered when no timer; below it the timer gray, to the minute: `12m`, `1h05`, an alarm `07:30`; one timer at a time
- tap: set a timer for 5 s, крутилка справа, ряд 3 sets it: 1 min per detent, a fast continuous spin speeds up every 4 detents to 2 / 5 / 10 / 15 / 30 / 60 min (the value snaps to the step); a pause or reversing drops back to 1 min, the value white; a second tap, the knob press or 5 s idle starts it (replacing the running one), 0 = nothing (under the clock the line stays empty at 0; it shows from 1m)
- turn left from 0: an alarm instead, time of day from the next 5 min, 5 min per detent, speeding up the same way
- hold ≥ 400 ms: cancel the timer
- due: the timer is cleared, the tile blinks white and Glass plays every 2 s for 1 min; a tap silences it
- timers survive a daemon restart (`~/.config/zored-stream/timers.json`), a missed one rings on start

## Buttons

LED color = state. In choice mode (below) кнопки 1–8 answer Claude instead.

| Button | Does | LED |
|---|---|---|
| кнопка 1 | KVM → Windows: press = Wake-on-LAN to the PC, then monitor input DisplayPort (`m1ddc set input 15`), the monitor moves USB and the controller with it; back only from Windows or the monitor menu. Hold = toggle PIP | pink, white 0.3 s on press |
| кнопка 2 | in a call: tap hides / shows the call layer (тайлы ряд 3, колонки 1–3 and кнопки 3/4/5 show and act as outside a call; the call goes on); hold ≥ 300 ms shows the other layer until release; a new call starts shown | blinks dim white while a call goes on, off outside a call |
| кнопка 3 | in a call: mic, tap toggles, hold ≥ 300 ms = push-to-talk (same as тайл ряд 3, колонка 1); otherwise free | in a call: red (dims 1 s every 4 s) = on, dim white = off, yellow = switching, blinking yellow = unknown; off = no call |
| кнопка 4 | camera toggle in a call | as кнопка 3 |
| кнопка 5 | outside a call: the calendar of тайл ряд 3, колонка 3, tap = open the list / act on the item; in a call: hang up | white, green = browsing; white in a call |
| кнопка 6 | free | off |
| кнопка 7 | dictation into Claude Desktop (below) | white = ready, blinking green = recording, off = no Claude window with a record button |
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
  the model rotated (reads bottom-up), effort as gray dots (low 1 … max 5), on a change new dots slide out of the previous one and slide back in; a red dot in the corner = the last fetch failed
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
- hold 3 s without a turn: the picture turns 90° clockwise, then again every 2 s while held: device orientation 90° → 0° → 270° → 180° → 90° (how far the device itself is turned clockwise; 90° by default), a haptic tap on each switch; survives restarts.
  90°: tile pictures turn 90° counter-clockwise, the left strip (track) 180°, the right strip stays. 180°: everything 180°. 270°: tiles 90° clockwise, the right strip 180°, the left stays.
  Taps and strip slides follow the picture, tile swipes follow the turned device. The tiles keep their places and controls: only the picture turns. The emoji picker turns with the device ([emoji.md](emoji.md#orientation))
- shows: the left strip bar for 1 s, white = Mac, gray = controller

[macos.md](macos.md)

### крутилка справа, ряд 2 — agent chats, AI model + effort

- turn: walks the agent chats (see тайл ряд 2, колонка 4); click: same as that tile
- while the chats browse (тайл ряд 3, колонка 1): turn walks the messages of the shown chat

- press + turn: walks Sonnet·Low … Sonnet·High → Opus·Low … → Fable·High (9 stops, stops at the ends)
- applies 400 ms after the knob rests, only on the Claude Desktop new-chat screen in front;
  elsewhere it previews for 5 s (gray label, the strip shows that model's limits), then shows the real model again

[ai-switch.md](ai-switch.md)

### крутилка справа, ряд 3 — scroll

- click (press and release without a turn): switch wheel ↔ tab mode; the mode survives restarts
- wheel mode, turn: mouse wheel under the cursor, clockwise = down; 1 / 2 / 4 / 8 lines per detent by speed
- tab mode, turn: Ctrl held, clockwise = Tab, counter-clockwise = Shift+Tab; Ctrl is released 500 ms after the last detent
- press + turn: wheel mode = horizontal wheel, clockwise = right; tab mode = Cmd held while pressed, clockwise = Cmd+Tab, counter-clockwise = Cmd+Shift+Tab (the system app switcher), release picks; for 3 s after that (extended by each detent) a turn without press = Cmd+` / Cmd+Shift+` through windows of the picked app, then back to Ctrl+Tab; no mode switch
- while the chats browse (тайл ряд 3, колонка 1): turn scrolls the shown message, press opens the chat
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
| volume, track, seek knobs, left strip, play tile, clock, brightness, scroll | same; no Yandex Music hold or swipes up/down; brightness = monitor over DDC/CI |
| mic tile + кнопка 2 | system mic mute (proposal) |
| keyboard layout, wi-fi, headphones, keep-awake, emoji, camera, day / hang-up tiles, timers, кнопки 1, 3, 4, 7 | dropped, free (the clock stays, no timers) |
| agents tile | Codex sessions |
| right strip, крутилка справа, ряд 2 | Codex limits; model switch later (the knob is still on the windows page, its daemon module isn't) |
| choice mode | off (no Claude Desktop) |


## On screen

`make screen` installs a panel at the top center of the primary display. It appears while the USB controller is unplugged
and slides away when the controller is plugged in. Companion draws the same page on both surfaces.

The row contains 12 tiles in column order: tiles 1–3 are the first device column, 4–6 the second, 7–9 the third, 10–12 the fourth.
The horizontal strips sit at the ends. On a MacBook with a camera cutout, tiles 1–6 sit to its left and 7–12 to its right; the strips remain at the outer ends. Displays without a cutout keep one continuous row. Hover smoothly enlarges nearby controls; changing apps or leaving the panel returns them to their normal size. The panel keeps keyboard focus in the current app.

- Click or hold a tile as on the device; drag at least 40 pt within 400 ms for its swipe action.
- Wheel over tiles 1–3 or 10–12 turns the corresponding left or right knob; right click presses that knob.
- Middle click on tiles 1–8 presses кнопка 1–8. There is no separate button row.
- Drag the left strip for volume, the right strip for model/effort. Wheel over the left strip changes volume by 2 % per step.
  While the emoji picker is open, strip press/drag/release follows the device picker controls.
- Double click either strip to slide the row upward out of view. Hold the cursor at the primary display's top edge for 0.6 s: the row starts peeking at once, 10 pt by the end, then comes back. Moving away earlier sends it back up. Plugging or unplugging the controller clears this hidden state.

Manual checks of the panel are pending. Technical details and the verification list: [screen.md](screen.md).
