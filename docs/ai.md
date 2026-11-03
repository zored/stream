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

- Fetch on start, then every `-usage-every`. Network/HTTP errors back off: interval × 2ⁿ, at most 1 h, at least `Retry-After`.
  Keychain problems (no item, no refresh token) retry every minute: a local read, no network.
- `GET /usage`: `{"limits": [...as upstream...], "fetched_at", "error", "next_at"}` from the cache, never fetches.
- `POST /usage/refresh`: fetch now, ignored within a minute of the last fetch.
- Variables: `ai_strip_svg` (right strip top cell), `ai_session`, `ai_session_reset`, `ai_day`, `ai_week`, `ai_week_reset`, `ai_level`
  (`low` < 50, `normal`, `high` > 80, `critical` > 90, from `ai_day`). The strip pictures are re-rendered with the 10 s resend, so time left stays current.
- `d` is the weekly percent scaled to the workday: `5 × percent / workdays since the window start` (Mon–Fri, 1…5), as in the overlay.

## Right strip

No tile (removed 2026-11-02). The whole right strip is one 48×258 SVG (the drawable area, see [companion.md](companion.md)) from the daemon (`ai_strip_svg`, `aiStripSVG`); each
cell shows its third (`gen_pages.py` `ai_strip`). A white bar fills the strip bottom-up like the volume bar; on top of it the
rotated model + effort dots and a value near the bottom (baseline 8 px above the bottom, `stripTextY`), gray like the date. Everything is drawn twice — normal, and black clipped to the bar — so the
text inverts where the bar is under it (verified in the web preview: full bar → black `opus ●`).

| When | Text | Bar |
|---|---|---|
| default | time left in the 5h session, `1:42` | session usage |
| while the model is switching (knob or strip) | the word `day` | `d` of the selected model |
| after the switch: while the label is in `preview`, or 5 s after an applied switch | whole days left in the week, `3d` (`h:mm` under a day) | `w` of the selected model |

The selected model uses its `weekly_scoped` limit when one exists (`display_name` starts with the model), else `weekly_all`.
Bars over 100 % stay full. The bar and the effort dots ease to new values at 60 fps (`animateStrip`, `stripHz`), so a change takes ~0.2 s. A red dot in the corner means the last fetch failed.
Stale data is never shown as current: once a limit's `resets_at` has passed, its value and bar are cleared; when the last fetch
failed or is older than 2 intervals, the bar is gray instead of white.

## Hammerspoon overlay

`~/.hammerspoon/usage_overlay.lua` reads `GET http://127.0.0.1:47116/usage` every minute instead of calling the API itself,
so one fetch serves both.
