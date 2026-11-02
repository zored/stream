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
- The daemon never calls the `claude` CLI and never refreshes the OAuth token: refreshing would rotate the refresh token under
  Claude Code. An expired token is an error until Claude Code is used again and refreshes it.
- Network: `api.anthropic.com` resolves to `0.0.0.0` here; the system PAC (`jbproxy.vk.team/aiproxy/wpad.dat`) sends it to
  `squid.ai-proxy.vk.team:8083`. Go ignores PAC, so the LaunchAgent passes `-usage-proxy` (Makefile `USAGE_PROXY`, verified).

## Daemon

| Flag | Default | |
|---|---|---|
| `-usage` | `127.0.0.1:47116` | HTTP address |
| `-usage-every` | `15m` | fetch interval |
| `-usage-proxy` | empty | HTTP proxy for `api.anthropic.com` |

- Fetch on start, then every `-usage-every`. Errors back off: interval × 2ⁿ, at most 1 h, at least `Retry-After`.
- `GET /usage`: `{"limits": [...as upstream...], "fetched_at", "error", "next_at"}` from the cache, never fetches.
- `POST /usage/refresh`: fetch now, ignored within a minute of the last fetch.
- Variables: `ai_limits_svg` (tile picture), `ai_session`, `ai_session_reset`, `ai_day`, `ai_week`, `ai_week_reset`, `ai_level`
  (`low` < 50, `normal`, `high` > 80, `critical` > 90, from `ai_day`). `ai_limits_svg` is re-rendered with the 10 s resend, so time left stays current.
- `d` is the weekly percent scaled to the workday: `5 × percent / workdays since the window start` (Mon–Fri, 1…5), as in the overlay.

## Tile

Above the clock (`1/5`), no actions, no text: six vertical bars filled bottom-up, side by side inside the 6% inset, left to right:

1. time left in the 5h session (full while the session is inactive)
2. session usage
3. `d`, all models
4. `w`, all models
5. `d`, the scoped limit (Fable)
6. `w`, the scoped limit

Bars are white, yellow > 80, red > 90 (over 100 % stays a full red bar); no green. A red dot in the corner means the last
fetch failed and the bars are old.

## Hammerspoon overlay

`~/.hammerspoon/usage_overlay.lua` reads `GET http://127.0.0.1:47116/usage` every minute instead of calling the API itself,
so one fetch serves both.
