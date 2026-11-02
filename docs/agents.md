# Agents waiting for me

Claude Code sessions (Desktop Code tab and CLI) that need an answer or have finished a turn, shown on the MEDIA page.
**Verified** = observed on this Mac on 2026-11-02, **assumed** = read, not checked.

## Controls

- кнопка 6 LED: amber breathing (2 s, 20 Hz from the daemon) while any session needs input; dim gray while only finished ones wait; off when none.
- тайл ряд 2, колонка 4: one dot per waiting session, amber = needs input, gray = done, input first; a number above 8.
- Press either: focus the next waiting session (needs input first, oldest first), the next press moves on and wraps.
  Focusing a "done" session drops it; "needs input" stays until the agent resumes.

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
| `PostToolUse` | needs input → removed (approved and running) |
| `UserPromptSubmit`, `SessionStart`, `SessionEnd` | removed |

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
