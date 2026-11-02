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

Knob **`1/7`** (right middle), strip **`0–2/6`** next to it.

- **One list, turn walks it**: `sonnet·low`, `sonnet·medium`, `sonnet·high`, `opus·low` … `fable·high` — 9 stops (Opus in the middle is
  an assumption, the user named the range "sonnet-fable"). Short enough for one list, no press+turn mode.
- Ends of the list: the existing end-of-range haptic; crossing to the next model: a stronger tick.
- Apply after the knob rests 400 ms, only when the guard passes: Claude frontmost and the main web area titled `Claude`.
  Otherwise nothing is typed and the strip shows the choice dimmed with `open a new chat`.
  - model changed: `AXPress` the `Model:` popup, `AXPress` the `AXMenuItem` whose title starts with the model name.
  - effort changed: `⌘⇧E`, `AXPress` the item whose title starts with the level.
  - read back the popup description; mismatch → strip red + error haptic.
- Strip shows the truth from the popup description (`Fable 5.1 · High`), polled while Claude is frontmost; press on the knob
  re-applies the shown choice (e.g. after a new-chat screen opened with the app's default).
- Press could also open the new-chat screen: `open claude://code/new` (verified; with `folder=` the app asks to trust the workspace).

`zored_stream` module `ai` (new, `127.0.0.1:47116`): `POST /ai/turn?d=±1`, `/ai/press`; variables `ai_label`, `ai_state`
(`ok`/`pending`/`error`), `ai_edge` for the haptic. AX in C next to `ax_darwin.m` (find by role + title prefix, press), keys via
`CGEventPost` (`⌘⇧E` only). The daemon already holds its own Accessibility grant.

Optional later: the same knob also writes CLI defaults for new sessions (`~/.claude/settings.json` `model` + `effortLevel`,
`~/.codex/config.toml` `model` + `model_reasoning_effort`, both verified to be read at start).

## Blockers

- Keystrokes in `zored_stream`: only `⌘⇧E`, plus the AX press helpers; nothing else.
- Fragile by nature: titles, badges and the `⌘⇧E` shortcut come from the app's UI and change with its updates; the read-back catches it.

