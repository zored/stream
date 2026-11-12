# AI limits

Claude plan usage on the MEDIA page, fetched by `zored_stream` (`tools/zored_stream/usage.go`).
**Verified** = observed on this Mac on 2026-11-02, **assumed** = read or recalled, not checked.

## Source

`GET https://api.anthropic.com/api/oauth/usage` with `Authorization: Bearer <token>` and `anthropic-beta: oauth-2025-04-20`.
The token is `claudeAiOauth.accessToken` from the keychain item `Claude Code-credentials`, read with `/usr/bin/security` on every fetch.
The response has `limits[]`: `kind` (`session`, `weekly_all`, `weekly_scoped`), `percent`, `resets_at`, `scope.model.display_name`
(verified).

- Cost: a plain read, no model call; the percentages did not move across several fetches in 10 minutes and the response carries
  no rate-limit headers (verified). That it never counts against the plan is assumed.
- The daemon never calls the `claude` CLI. The CLI refreshes its token only when it runs; the user mostly works in Claude Desktop,
  so the token used to expire and the limits froze (2026-11-03: expired 8 h, `claude auth status` does not refresh it, verified).
- Token refresh (`claude_token.go`, user-approved 2026-11-03): when the access token is expired or within 5 min of it, or after a
  `401` from the usage endpoint, the daemon does what Claude Code 2.1.316 does (read in its binary): `POST
  https://platform.claude.com/v1/oauth/token` `{"grant_type":"refresh_token","refresh_token","client_id":"9d1c250a-e61b-44d9-88ed-5944d1962f5e","scope":<scopes joined by spaces>}`,
  then writes `accessToken`, `refreshToken`, `expiresAt`, `refreshTokenExpiresAt` back into the same keychain item the same way:
  `security -i` ← `add-generic-password -U -a <user> -s "Claude Code-credentials" -X <hex JSON>`, every other field kept
  (`mcpOAuth`, `scopes`, `subscriptionType`…). Verified: the access token lives 8 h and the refresh token **rotates**, so a private
  copy would have logged the CLI out; after the write the item kept all fields and the new token fetched the limits.
  Not covered: Claude Code's own write lock (`.storage-write`); a CLI refreshing at the same second could lose the race (assumed rare).
- Claude Desktop keeps its own token encrypted with Electron safeStorage (`oauth:tokenCacheV2` in `config.json`); not used.
- Network: `api.anthropic.com` resolves to `0.0.0.0` here; the system PAC (`jbproxy.vk.team/aiproxy/wpad.dat`) sends it to
  `squid.ai-proxy.vk.team:8083`. Go ignores PAC, so the LaunchAgent passes `-usage-proxy` (Makefile `USAGE_PROXY`, verified).

## Daemon

| Flag | Default | |
|---|---|---|
| `-usage` | `127.0.0.1:47116` | HTTP address |
| `-usage-every` | `15m` | fetch interval |
| `-usage-proxy` | empty | HTTP proxy for `api.anthropic.com` |

- Fetch on start, then every `-usage-every`. HTTP errors back off: 1 min × 2ⁿ⁻¹, at most 1 h, at least `Retry-After`.
  Keychain problems (no item, no refresh token) retry every minute: a local read, no network.
  No route (`dial`/`proxyconnect` errors: the VPN is off and `squid.ai-proxy.vk.team` does not resolve) also retries every minute
  and does not grow the backoff: nothing reaches the server. Before 2026-11-10 it backed off to 1 h, so the red dot stayed up to
  an hour after the VPN came back (verified in the log).
  A client timeout (20 s, no headers) retries every minute too. Before 2026-11-12 it counted as a server error and the first
  backoff was 30 min: one slow request right after a daemon restart left the strip on a red dot with no limits for half an hour
  (verified in the log, 22:53 → next try 23:23).
- `GET /usage`: `{"limits": [...as upstream...], "fetched_at", "error", "next_at", "context"}` from the cache, never fetches.
  `context` = `{"tokens", "window", "percent", "model"}` of the active chat (agents ring), see [Context](#context).
- `POST /usage/refresh`: fetch now, ignored within a minute of the last fetch.
- Variables: `ai_strip_svg` (right strip top cell), `ai_session`, `ai_session_reset`, `ai_day`, `ai_week`, `ai_week_reset`, `ai_level`
  (`low` < 50, `normal`, `high` > 80, `critical` > 90, from `ai_day`). The strip pictures are re-rendered with the 10 s resend, so time left stays current.
- `d` is the weekly percent scaled to the workday: `5 × percent / workdays since the window start` (Mon–Fri, 1…5), as in the overlay.

## Right strip

No tile (removed 2026-11-02). The whole right strip is one 48×258 SVG (the drawable area, see [companion.md](companion.md)) from
the daemon (`ai_strip_svg`, `aiStripSVG`); each cell shows its third (`gen_pages.py` `ai_strip`). Since 2026-11-12, full bleed:

- left, 32 px: the 5 h session bar, bottom-up.
- right, 16 px: the weekly bar of the selected model: its `weekly_scoped` limit when one exists (`display_name` starts with the
  model, today only Fable), else `weekly_all`. Behind it a dark gray bar up to how far the week may be by the end of today:
  `100 × workdays since the window start / 5` (Mon–Fri, 1…5). There is one 5 h limit for all models (verified: no scoped `session`).
- colours as the Hammerspoon overlay: < 50 light green `#99F299`, ≤ 80 white, > 80 yellow `#FFD94D`, > 90 red `#FF6666` (5 h bar,
  by its %). The weekly bar goes by `d` (its % of today's allowance, `weekLevelOf`): < 50 light green, ≤ 80 white, > 80 red
  `#FF6666` (near today's limit), > 100 dark red `#B3261E` (over it).
- the rotated model + effort dots sit over the 5 h bar: white outside it, black where the bar is under them. On the same line,
  at the bottom edge, time left in the 5 h window (`1:20`); in the weekly bar, at the bottom, time left in the week (`1d 5h`,
  `5h` under a day, `40m` under an hour). Same inversion; over a dark red weekly bar the text stays white.
- no animation: a model or effort change redraws at once.

Bars over 100 % stay full. A red dot in the corner means the last fetch failed. The calendar list covers the whole strip while open (`usage.cover`, see controls.md, clock).
Stale data is never shown as current: once a limit's `resets_at` has passed, its bar is cleared; when the last fetch
failed or is older than 2 intervals, both bars are gray.

## Context

The agents tile (тайл ряд 2, колонка 4) draws a 4 px bar along its top edge: how full the context of the ringed chat is
(`context.go`). Source: the chat's transcript `~/.claude/projects/*/<id>.jsonl` (`id` = `cliSessionId` for Desktop, the hook
`session_id` for CLI), last 512 KB, the newest `message.usage`: `input_tokens + cache_read_input_tokens + cache_creation_input_tokens`
(verified 2026-11-12: matches the request that produced it). Re-read only when the file's size or mtime changes.
Window: 1M, 200k for Haiku. Assumed from the transcripts: Opus 5.5 reached 627k, Fable 5.1 848k, Sonnet 5.5 213k, Haiku 5.5
at most 120k; no file states the window. Same colours as the strip.

## Hammerspoon overlay

`~/.hammerspoon/usage_overlay.lua` reads `GET http://127.0.0.1:47116/usage` every minute instead of calling the API itself,
so one fetch serves both. A last line `ctx: 21%` shows `context.percent` when the daemon has one.
