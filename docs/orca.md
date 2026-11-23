# Orca workspaces and chats

тайл ряд 2, колонка 4 and the right knobs browse the workspaces and chats of [Orca](https://orca.stablyai.com) (`com.stablyai.orca`, 1.4.271).
Module `orca` of `zored_stream` (`tools/zored_stream/orca.go`, `orca_darwin.go`). User side: [controls.md](controls.md#тайл-ряд-2-колонка-4--orca-workspaces).
**Verified** = observed on this Mac, **assumed** = read, not checked.

## Source

- `orca worktree ps --json` (`/Applications/Orca.app/Contents/Resources/bin/orca`, ~0.5 s wall, ~0.25 s CPU, verified 2026-11-23): `result.worktrees[]` with `worktreeId` (`<repoId>::<path>`), `displayName`, `workspaceKind` (`git` / `folder`), `isActive`, `isArchived`, `agents[]` with `orcaSessionId`, `conversationName`, `prompt`, `lastAssistantMessage` (cut to 200 chars, verified), `activity {listed, status, unread, sortAt, title}`.
- Order: a workspace by the newest `activity.sortAt` of its chats; `lastActivityAt` lags by minutes (verified by Robbo). Chats by `sortAt`. Unlisted chats and archived or chatless workspaces are left out.
- Chat icon on the card: the emoji the chat name starts with, else the logo by `agentType` (`claude`, `codex` → OpenAI; Bootstrap Icons 1.13.1 `claude.svg`, `openai.svg`), white on the dark disc. Verified 2026-11-23 on the tile picture (Claude and OpenAI logos).
- Poll every 15 s, and once when a browse opens. The shown chat's full last reply: `orca agent read orca_session_id:<id> --json` → the last `entries[]` item with `kind: message`, `role: assistant` (~0.3 s, verified 2026-11-23: 576 chars where `ps` had 200); крутилка справа, ряд 1 scrolls it; past its ends the push bar of the chats browse moves to the next / previous chat, through the workspaces (since 2026-11-24 one detent of bar, the second moves, `chatsPushTurns` = 2, a line per scroll detent; verified 2026-11-23 with 3 detents on the strip picture: top bar at 2/3, the last detent back landed at the end of the previous workspace's last reply).

## Open a chat

No CLI command or deeplink opens a chat. The chat tab in the Orca window is an `AXButton` whose `AXTitle` is `[<status> ]<chat> Close tab <chat>` (no status prefix on some tabs, verified 2026-11-23 on `devcontainer #1`); its close button is another `AXButton` with `Close tab <chat>` only in `AXDescription`, so the match is on `AXTitle` alone and never closes a tab. `AXPress` switches to the chat. Only the tabs of the active workspace are in the tree.

1. Orca to the front: `LSOpenCFURLRef` on the app bundle. It must come first: the reopen switches Orca back to the workspace it last had in front, undoing an `activate` made before it (verified 2026-11-23). `activateWithOptions:` from the daemon returned NO (verified).
2. Git workspace: `orca worktree activate --worktree id:<worktreeId> --json` (~0.3 s, verified). Folder workspaces cannot be activated from the CLI: the daemon answers `409` unless that workspace is already active.
3. Poll the AX tree every 100 ms for 6 s for the tab (title suffix `<chat> Close tab <chat>`), `AXPress`. A miss logs the window count, nodes walked and every button title + description seen.

Verified 2026-11-23 through the daemon's HTTP routes: a chat in the active workspace (299 ms) and in another one (`razer` → `macos` 239 ms, `macos` → `razer` 393–424 ms), seen on screen by Robbo. On a locked screen (`CGSSessionScreenIsLocked`) Orca's `AXWindows` holds only the app element and the tree is the menu bar (166 nodes): the open fails with that log line.
