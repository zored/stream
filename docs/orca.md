# Orca workspaces and chats

тайл ряд 2, колонка 4 and the right knobs browse the workspaces and chats of [Orca](https://orca.stablyai.com) (dev build `com.stablyai.orca.dev`, profile `~/Library/Application Support/orca-dev-copy`, 1.4.272).
Module `orca` of `zored_stream` (`tools/zored_stream/orca.go`, `orca_darwin.go`). User side: [controls.md](controls.md#тайл-ряд-2-колонка-4--orca-workspaces).
**Verified** = observed on this Mac, **assumed** = read, not checked.

## Source

- `orca worktree ps --json` (`/Applications/Orca.app/Contents/Resources/bin/orca` with `ORCA_USER_DATA_PATH` = the dev profile; without it the CLI talks to the release app, verified 2026-11-27; ~0.5 s wall, ~0.25 s CPU, verified 2026-11-23): `result.worktrees[]` with `worktreeId` (`<repoId>::<path>`), `displayName`, `workspaceKind` (`git` / `folder`), `isActive`, `isArchived`, `agents[]` with `orcaSessionId`, `conversationName`, `prompt`, `lastAssistantMessage` (cut to 200 chars, verified), `activity {listed, status, unread, sortAt, title}`.
- Paired hosts (a devcontainer over `--environment`, e.g. `integrator container`): the local `ps` leaves them out and names them in `result.hostScope.omittedHostSelectors[].selector` (`--environment kaleidoscope-ide`); each is read with `orca worktree ps <selector> --json` in parallel (~4 s, verified 2026-11-25). Their agents carry no `activity`: all count as listed and read, time = `stateStartedAt`. `agent read` gets the same selector. A paired chat opens by its sidebar row like any other (verified).
- Tile cards are chats since 2026-11-25 (`orcaChat.entry`): idle the 4 freshest of the flat list, while browsing the shown chat (before: workspace cards, so 13 chats looked like 5).
- Order: a workspace by the newest `activity.sortAt` of its chats; `lastActivityAt` lags by minutes (verified by Robbo). Chats by `sortAt`. крутилка справа, ряд 3 walks one flat list of all chats by `sortAt`, across workspaces, in a loop (since 2026-11-25). While browsing, a poll never moves the shown chat: it is kept by id though the order changed, a new reply of it keeps the scroll; a gone chat closes the browse. Opening a chat keeps the browse on it. A reply's trailing line starting with `TLDR` / `TL;DR` (markdown marks allowed) moves, with what follows, to the start of the strip text without its label (`TLDR:`, `**TL;DR** —`), two empty lines below it (`orcaReply`). The reply's markdown is formatted (`orcamd.go` → `cal_canvas_rich`, verified 2026-11-25 on a rendered strip): line breaks kept, blank lines and rules dropped, headings and `**strong**` bold white, `` `code` `` and fenced blocks white, bullets `•` (nested indented), quotes `│`, table rows `a · b` without the separator row, links keep their text, `*em*` loses its marks; word wrap. Unlisted chats and archived or chatless workspaces are left out.
- Chat icon on the card: the emoji the chat name starts with, else the logo by `agentType` (`claude`, `codex` → OpenAI; Bootstrap Icons 1.13.1 `claude.svg`, `openai.svg`), white on the dark disc. Verified 2026-11-23 on the tile picture (Claude and OpenAI logos).
- Poll every 15 s, and once when a browse opens. The shown chat's full last reply: `orca agent read orca_session_id:<id> --json` → the last `entries[]` item with `kind: message`, `role: assistant` (~0.3 s, verified 2026-11-23: 576 chars where `ps` had 200); крутилка справа, ряд 1 scrolls it; past its ends the push bar of the chats browse moves to the next / previous chat of the flat list (since 2026-11-24 one detent of bar, the second moves, `chatsPushTurns` = 2, a line per scroll detent; verified 2026-11-23 with 3 detents on the strip picture: top bar at 2/3, the last detent back landed at the end of the previous workspace's last reply).

## Open a chat

No CLI command or deeplink opens a chat. Since 2026-11-25 the daemon presses the chat's row in the sidebar outline (`AXOutline` «Agents»): an `AXRow` titled `[<status> ]<chat> - <preview> <age>`, present for every workspace and paired host. `AXPress` switches workspace and tab in one go, folders and pinned tabs included.

1. Orca to the front: `LSOpenCFURLRef` on the app bundle (`activateWithOptions:` from the daemon returned NO, verified).
2. Poll the AX tree every 100 ms for 6 s for the row (title contains ` <chat> - `, else starts with `<chat> - `), `AXPress`. A miss logs the window count, nodes walked and every button title + description seen.

Dead ends (verified 2026-11-25): `orca worktree activate --environment <host>` succeeds but the UI stays on the old workspace; pinned tabs have no `Close tab` in their title, so the old tab match (`<chat> Close tab <chat>`) missed them. Verified 2026-11-25 through `/agent/focus`: local `integrator local` chat 554 ms, paired `integrator container` chat 85 ms, row press desktop → `aqa ex` (remote, pinned) and back → `retro`, seen on screen.

## Close a chat

Tile release after 1.5 s (`/agent/close`, only while browsing): row press, then the tab's close button (`AXButton`, `AXDescription` = `Close tab <chat>`), no confirmation for a done chat (verified 2026-11-25 on an empty chat from `orca agent launch --agent claude --surface chat`; a working chat: assumed). A pinned tab has no close button: the press fails, logged.

## Mark read

No CLI command marks a chat read. The sidebar row's context menu (`AXShowMenu` on the `AXRow`) holds `Sleep`, `Mark Read` / `Mark Unread`, `Mute Notifications`, `Edit Details…`, `Pin`, … for that chat; an unread row's title carries `<status> · Unread <chat> - …`. Tile hold presses `Mark Read` (`orcaMarkRead`); verified 2026-11-25: `devcontainer skill` and `maturity` turned `activity.unread: false` (333 ms through `/agent/read`).

Earlier, tab-based route: Verified 2026-11-23 through the daemon's HTTP routes: a chat in the active workspace (299 ms) and in another one (`razer` → `macos` 239 ms, `macos` → `razer` 393–424 ms), seen on screen by Robbo. On a locked screen (`CGSSessionScreenIsLocked`) Orca's `AXWindows` holds only the app element and the tree is the menu bar (166 nodes): the open fails with that log line.
