# Agents waiting for me

Claude Code sessions (Desktop Code tab and CLI) that need an answer or have finished a turn, shown on the MEDIA page.
**Verified** = observed on this Mac on 2026-11-02, **assumed** = read, not checked.

## Controls

- кнопка 6 LED: amber breathing (2 s, 20 Hz from the daemon) while any session needs input; dim gray while only finished ones wait; off when none.
- тайл ряд 2, колонка 4: one dot per session: amber = needs input, white = done, gray = working, in that order; the last focused one has a white ring; a number above 8; a hollow gray circle when none.
- Press either: focus the next session (needs input, then done, then working, oldest first), the next press moves on and wraps.
  Focusing a "done" session drops it; "needs input" stays until the agent resumes. With none waiting, a press opens a new Code session (`claude://code/new`). Holding the tile or кнопка 6 (400 ms) always opens a new Code session; both focus on release.

## Flow

Global hooks in `~/.claude/settings.json` post the hook JSON to `zored_stream` (`agents.go`, `127.0.0.1:47120`):

```sh
curl -s -m 1 --noproxy "*" -o /dev/null -H "Agent-Pid: $PPID" -H "Agent-Host: ${CLAUDE_CODE_HOST_SESSION_ID-}" \
  -H "Agent-Iterm: ${ITERM_SESSION_ID-}" -H "Agent-App: ${__CFBundleIdentifier-}" \
  --data-binary @- http://127.0.0.1:47120/agent/event || true
```

`async: true`, exit 0 always: a stopped daemon costs nothing. `--noproxy` is needed: hooks inherit the app's proxy env,
and without it the post went to the corporate proxy and was lost (verified).

| Hook | State |
|---|---|
| `PermissionRequest`, `Notification` `permission_prompt` / `elicitation_dialog`, `PreToolUse` `AskUserQuestion` | needs input |
| `Stop`, `StopFailure`, `Notification` `idle_prompt` | done |
| `UserPromptSubmit`, `PostToolUse` | working |
| `SessionStart`, `SessionEnd` | removed |

Expiry every 10 s: the hook's `$PPID` is the `claude` process (verified), a dead pid drops the session; `-agents-ttl` (4 h) is the backstop.

Variables: `agents_led` (LED color as an int), `agents_svg` (tile). `GET /agents` lists the state, `POST /agent/focus` is the press (`409` when empty).

## Which hooks fire

- CLI interactive (verified with `expect` + `--settings`): `SessionStart`, `UserPromptSubmit`, `PermissionRequest` and
  `Notification permission_prompt` on a permission prompt, `PostToolUse` after approval, `Stop`, `SessionEnd`. Rejecting with Esc fires no `Stop`.
- CLI `-p` (verified): the same minus the permission ones.
- Desktop Code tab: runs the same `claude` with the user settings (`SessionStart` hooks seen in its sessions, verified);
  `CLAUDE_CODE_HOST_SESSION_ID=local_…` and `CLAUDE_CODE_ENTRYPOINT=claude-desktop` in the env (verified).
  Which events fire on a Desktop permission card: see the status below.

## Focus

- Desktop: `open claude://code/continue?session=local_…`. The app's URL handler takes `/continue?session=` with a `local_` id
  and routes to that session (read in `app.asar`; also `/needs-input`, `/new`). It is ignored when the app's "entry points" setting is off.
- iTerm2: `ITERM_SESSION_ID` `w0t1p0:<uuid>`, AppleScript selects the window, tab and session by `unique id`.
  The first press asks for Automation permission (`zored_stream` → iTerm2).
- Anything else: `open -b $__CFBundleIdentifier` activates the app (no tab).

Setup: `make agent-hooks` (`tools/agent_hooks.py`) adds the hooks to `~/.claude/settings.json`, rerunning replaces them
(entries posting to `/agent/event` are removed first). State lives in memory: restarting `zored_stream` forgets waiting sessions.

## Next: Codex

Codex 0.153 here (verified): `notify` in `~/.codex/config.toml` runs one program with the event JSON as the last argument,
only `agent-turn-complete` (assumed, from the docs); the slot is taken by Computer Use (`SkyComputerUseClient turn-ended`), `[features] hooks = false`.

1. `tools/codex_notify.sh`: runs the Computer Use client with the same arguments, then `curl` to `POST /agent/codex` with the JSON; `make codex-hooks` points `notify` at it.
2. Daemon: `/agent/codex` maps `agent-turn-complete` to done, keyed by `thread-id`, with `cwd`, `TERM_PROGRAM`/`ITERM_SESSION_ID` from the env.
3. Clearing: notify has no prompt-submit event, so a Codex entry clears on focus, on its pid dying, or on the TTL.
   Needs-input (approvals) only with Codex hooks: check what `hooks = true` offers (assumed: `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `Stop`)
   and reuse `/agent/event` if the JSON matches Claude's.

## Next: Windows

1. `agents.go` stays shared; the macOS parts move out: pid check (`syscall.Kill` → `OpenProcess` + `GetExitCodeProcess`) and focus.
2. Focus: Desktop `start claude://code/continue?session=local_…` (same handler, assumed); Windows Terminal has no per-tab focus
   by id, so activate the window (`wt -w 0 focus-tab` needs the tab index; assumed).
3. Hooks: Claude Code on Windows runs hooks through Git Bash when present (assumed), so the same `curl` line with `curl.exe` works; `agent_hooks.py` picks the variant by OS.
4. Pages unchanged: same port, variables and controls.

## Not done

- Haptic on a new waiting session: the module fork has `vibrate()` but no Companion action triggers it.
- Session titles: hooks carry no title; `GET /agents` shows the cwd basename.

## Dictation

кнопка 7 drives voice input in the current Claude Desktop chat. LED: red = ready, blinking green (1 Hz) = recording,
off = no Claude window with a record button (`dictation` = `off` / `on` / `none`).

| Gesture | Effect |
|---|---|
| hold, speak, release | push-to-talk: records while held, sends on release |
| tap | starts recording and keeps it on |
| tap while recording | stops and sends |
| hold while recording (≥ 400 ms) | stops, keeps the text in the composer, does not send |

Verified 2026-11-03 with `say` into the mic: all four, the sent ones arrived as messages in this session.

- The composer has an `AXCheckBox` (subrole `AXToggleButton`) described `Press and hold to record`, and `Stop dictation`
  while recording; `AXPress` toggles it. The transcript streams into the `AXTextArea` `Prompt` while recording and the
  last words land within ~0.5 s after the stop (verified). The first press after the app started once only set `AXValue` 1
  without recording (cause unknown); presses after that toggled cleanly.
- `zored_stream` (`dictate_darwin.go`, `POST :47117/dictation/down`, `/dictation/up`): the handlers only queue the event
  with its timestamp and return `204`; one worker applies them in order, so Companion's 1–2 s `curl`/exec timeouts never cut
  a send. Hold time is measured between the event timestamps. Send = wait until the prompt text is unchanged for 700 ms
  (4 s cap), focus the prompt (`AXFocused`), post Return to Claude, check the prompt emptied. An empty transcript is not sent.
  Errors go to the log.
- Claude is activated first when it is in the background. `dictation` (`on`/`off`) is read every second, so dictation
  started with the mouse lights the LED too.
- The bundle has `dictationShortcut` (`default` / `off` / an accelerator) in the app preferences, but the default key
  lives in the claude.ai frontend, not in `app.asar` (read); AX avoids depending on it.

## Choice mode

When the session on screen asks a question, кнопки 1–8 answer it; their own actions (mic, camera, hang-up, agents,
dictation) are suspended until the card is gone.

| Button | Single question, single choice | Several questions or multiple choice |
|---|---|---|
| кнопка 1 | skip | previous question; skip on the first |
| кнопки 2–7 | options 1–6: pick and go on | single choice: pick and go on; multiple: press toggles |
| кнопка 8 | option 7 | next question; finish on the last one |

LEDs: red = the button does something now, green = the option is picked (Claude Desktop preselects the first option of a
single choice, so it starts green), off = unused.

Picking the only choice of the last question submits the card.

- Gate: every row-3 button runs `curl -f :47120/choice/{down,up}?b=N || <its own action>`. The daemon answers `204` while a card is
  on screen (and for the matching release), `409` otherwise, so with the daemon down the buttons work as before.
  LEDs come from `choice_on` and `choice_1`…`choice_8` through the `box0` colour of each button: the LED follows the
  background layer only, an extra overlay layer did not light it (verified on the device).
- Detection every 500 ms, only while Claude Desktop or iTerm2 is frontmost (`choice.go`, `choice_darwin.go`);
  `GET :47120/choice` shows what the daemon sees and why choice mode is off.
- Frontmost app: `lsappinfo front` + `lsappinfo info -only pid` (~13 ms). Verified 2026-11-03 that the alternatives fail in the
  LaunchAgent: `NSWorkspace.frontmostApplication` never updates without an AppKit run loop (stayed on Claude), the system-wide
  `AXFocusedApplication` returns nothing, and the first layer-0 window of `CGWindowListCopyWindowInfo` belongs to
  Stage Manager (`WindowManager`) or another app's window (Preview) rather than the frontmost app.
- Claude Desktop (verified 2026-11-03 on live cards, pressed through Companion): `ax_card` finds the `Skip` button, walks up to the
  card and reads options (`AXCheckBox`/`AXToggleButton` with a title and no description, `AXValue` 1 = picked), `N of M`,
  `N selected` (only on multiple choice), `Next`/`Submit`, `Show previous question`. An option of a single-choice question
  ignores the first `AXPress` (it only takes focus); the daemon presses until `AXValue` is 1, at most 3 times, and fails loudly otherwise.
- CLI in iTerm2: options from the `PreToolUse`/`PermissionRequest` hook JSON (`tool_input.questions`); the daemon tracks the
  question and the picks itself and types into the session whose `unique id` is current (`write text … newline NO`).
  Keys verified in `tmux` 2026-11-03: a digit picks a single choice and goes on (submits a lone question), a digit toggles a
  multiple choice, `Tab`/`Shift-Tab` move, `Tab` past the last question opens the review where `1` submits, `Esc` cancels.
  Verified 2026-11-03 in iTerm2 with Opus: single choice went on to the next question, multiple-choice toggles, finish submitted.
  The first use asks for Automation permission (`zored_stream` → iTerm2).
- Claude Desktop next-step box (the recap with a question and reply buttons under the last message): кнопка 1 = `Dismiss`
  when the box has one, кнопки 2… = its buttons in order (the first, primary one white like in the app, the rest red), a press clicks it. Verified 2026-11-03: кнопка 3 clicked the second
  reply. AX: an `AXGroup` described `Suggested next actions…` next to the question text; replies are `AXButton`s with a title and
  no description. A box that waits on the user has no `Dismiss` button (anchoring on `Dismiss` missed it), so кнопка 1 stays
  dark there. A question card wins over a box.
- Permission prompts in the CLI: кнопка 2 = `1` (yes), кнопка 1 = `Esc` (not run yet). In Claude Desktop the permission card is not handled yet:
  its AX tree has not been seen (this session runs in auto mode).
