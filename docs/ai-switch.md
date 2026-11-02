# AI model + effort switch

One knob picks model and effort for Claude Code (CLI, Claude Desktop Code tab) and Codex.
**Verified** = observed on this Mac on 2026-11-02 (Claude Code 2.1.308, Codex 0.153.2, both run in `tmux` on copies of the settings),
**assumed** = read or recalled, not checked.

## Mechanisms

| App | Mechanism | Running session | New sessions | Status |
|---|---|---|---|---|
| Claude Code CLI | `model` / `effortLevel` in `~/.claude/settings.json` (or `.claude/settings.local.json`) | not picked up: file changed haiku→sonnet, `/status` still showed haiku | picked up (start banner "Haiku 5.5 with low effort") | verified |
| Claude Code CLI | type `/model <alias>` + Enter | switches at once | **also writes `model` into `~/.claude/settings.json`** ("saved as your default for new sessions"), even with a project-local `model` | verified |
| Claude Code CLI | type `/effort <level>` + Enter | switches at once, "this session only", no file written | no | verified (`max`) |
| Claude Code CLI | per-model effort `modelSettings.<id>.effortLevel` in `settings.json` | — | yes | assumed (key is in the user's settings) |
| Claude Desktop Code tab | model + effort pickers under the composer, per session | yes | defaults per preset in `~/Library/Application Support/Claude/ccd-session-presets.json` (`settings.modelId`, `settings.effort`) | pickers: assumed; presets file: verified (read) |
| Claude Desktop Code tab | type `/model …` / `/effort …` into the composer | the renderer handles both locally (`case"effort"`: `readEffortControl`, parses the level) | draft composer: unknown | handler read in the bundle; effect not tried |
| Claude Desktop Code tab | `⌘⇧E` opens the effort menu (Low, Medium, High, Extra, Max); pick by `AXPress` on the item or `↓…⏎` | yes | yes, on the new-chat screen | verified 2026-11-02; needs Claude frontmost; digits do **not** pick in this menu |
| Claude Desktop Code tab | `⌘⇧I` model menu | — | — | verified: does nothing here (gated by a flag in the code) |
| Claude Desktop Code tab | AX: `AXPopUpButton` with description `Model: <model> <effort>` → `AXPress` → `AXMenuItem` titles `Opus 5.5`, `Fable 5.1`, `Sonnet 5.5 Default`, `Haiku 5.5`, `Effort <level>`, `More models`; `AXValue` 1 = checked → `AXPress` the item | yes | yes | verified: Fable → Sonnet on the new-chat screen; effort is kept across a model switch; opening the popup while Claude is in the background was flaky |
| Claude Desktop | deep link `claude://code/new` | — | opens a new chat; accepts only `q`/`prompt`, `folder`, `file`, `ssh_*`, `environment`, `runner_pool`: no model or effort | read in the bundle |
| Claude Desktop | Settings toggles `always-start-with-default-model`, `always-start-with-default-effort-level` (private beta) | — | new chats start with a fixed default instead of the last used | read in the bundle; where the default is stored unknown |
| Claude Desktop | `set_session_model` / `set_session_effort` tools of the app's own MCP | yes, any session by id | — | verified to exist; callable only from inside a Claude session, no external API |
| Codex TUI | `model` / `model_reasoning_effort` in `~/.codex/config.toml` | not picked up: effort changed high→medium, `/status` still "reasoning high" | picked up | verified |
| Codex TUI | `/model` + Enter → numbered model list → Enter → numbered effort list (Low, Medium, High, Extra high, More reasoning… = max/ultra) → Enter | switches ("Model changed to gpt-6-astra high") | **writes both keys into `config.toml`** | verified |
| Codex TUI | `/model <name>` with an argument | — | — | assumed absent; the picker is the only path seen |
| Codex | `codex -m <model> -c model_reasoning_effort=<e>`, `[profiles.*]` + `--profile` | start only | yes | assumed (docs) |

Found on the way (verified):

- The `claude` shell function is a wrapper that refuses to run; the binary is `~/.local/bin/claude`.
- Every interactive Claude Code session listens on `uds:/tmp/cc-socks/<pid>.sock` (`/status` "Peer address"). It carries
  inter-session messages; whether it accepts a model/effort change is unknown (assumed no).
- `ccd-session-presets.json` is a mirror: the renderer keeps presets in a zustand store persisted to Local Storage,
  hydrates once and writes the file back (`fileMirrored`). External edits are not picked up while the app runs (assumed from the code).
- Accessibility: after the user granted it to Claude, its child processes are trusted (`AXIsProcessTrusted() = 1`), `osascript`
  still is not (it goes through System Events). Electron needs `AXManualAccessibility = true` on the app element to expose the web tree.
- Item titles carry badges (`High Recommended`, `Max 3.5× or more usage`, `Sonnet 5.5 Default`); the badge moves with the model, so match by prefix.
- Screen detection: the main `AXWebArea` title is `Claude` on the new-chat screen and `<session title> - Claude Code` in a session;
  both have `AXTextArea` `Prompt` and the `Model:` popup. Probe tools: `axdump`, `axpress`, `keys` (scratch, not in the repo).
- The probe's `/model sonnet` overwrote the user's `model = opusplan` in `~/.claude/settings.json`; restored right away.

## Design

Scope (user, 2026-11-02): only the **new-chat screen** of Claude Desktop, first priority; running sessions stay untouched.
Levels up to High, models Sonnet → Fable.

Knob **`1/7`** (right middle), label on the right strip (`0–2/6`) next to it. Implemented and verified 2026-11-02 through the
Companion API (`rotate-right`/`rotate-left` on `1/1/7`): Sonnet·Medium → Sonnet·High → Opus·Medium → back, the Desktop picker followed.

- **One list, turn walks it**: `Sonnet·Low`, `Sonnet·Medium`, `Sonnet·High`, `Opus·Low` … `Fable·High` — 9 stops (Opus in the middle is
  an assumption, the user named the range "sonnet-fable"). Clamped at the ends. No end-of-range haptic: the module fork only has
  press/hold/slide haptics.
- Apply after the knob rests 400 ms, only when the guard passes: Claude frontmost, its focused window has an `AXWebArea` titled `Claude`.
  - model changed: `AXPress` the `Model:` popup, `AXPress` the `AXMenuItem` whose title starts with `<model> `.
  - effort changed: `⌘⇧E` posted with `CGEventPostToPid` to Claude only (never to whatever app is in front), `AXPress` the item
    whose title starts with the level.
  - read back the popup description; mismatch → `Escape`, `ai_state = error`.
- Every second while nothing is pending and Claude is frontmost, `zored_stream` reads the popup in the focused window — the new-chat
  screen or an open session — and publishes the truth; the knob position follows it (verified 2026-11-02: an open Opus·Medium session
  showed `ok`). Applying stays new-chat only: a turn in an open session previews the choice for 5 s (gray label, the strip shows that
  model's limits, state `preview`), then the poll brings back the session's real model. Claude Code CLI is not synced (user: not needed).
- Turn while pressed: only the model changes, the effort stays. A press released without a turn re-applies the shown choice.
  Companion sends `down`/`up`/`turn` as separate `curl` processes, so API calls fired back to back can arrive out of order
  (seen in a test burst); by hand the press comes first.
- Label: `ai_label_svg` across the right strip, drawn like the track on the left strip (lowercase gray model `|` bold effort, `stripSVG`)
  but rotated 270° so it reads bottom-up; the effort is white `ok`, gray `pending`, dark gray `idle` (not on the new-chat screen),
  amber `other` (a model/effort outside the list, e.g. Haiku or Extra), red `error`.

`zored_stream` module `ai` (`ai_darwin.go`, `127.0.0.1:47117`, flag `-ai`): `POST /ai/turn?d=±1`, `POST /ai/down`, `POST /ai/up`; variables `ai_model`,
`ai_effort`, `ai_state`. AX helpers in `ax_darwin.m`: one BFS with a predicate, `ax_find_attr` (role + attribute, exact or prefix),
`ax_string`, `ax_focused_window`, `ax_frontmost_pid`, `ax_key`.

Not verified: the guard while a session (not the new-chat screen) is in front — the poll path showed `idle` there, the apply path was
not run; the look of the label on the device.

Optional later: the same knob also writes CLI defaults for new sessions (`~/.claude/settings.json` `model` + `effortLevel`,
`~/.codex/config.toml` `model` + `model_reasoning_effort`, both verified to be read at start).

## Risks

- Titles, badges and the `⌘⇧E` shortcut come from the app's UI and change with its updates; the read-back turns that into `error`.
- An early build posted `⌘⇧E` through the HID tap: if the user switched apps between the guard and the key, the key went to that app.
  Fixed with `CGEventPostToPid`.
