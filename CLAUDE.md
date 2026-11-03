# CLAUDE.md

Config-as-code for a Razer Stream Controller (= Loupedeck Live) driven by Bitfocus Companion, on macOS now, Windows next.
Status and next steps: `PLAN.md`.

## Docs — read before touching the area

- `docs/controls.md` — what every control does, user-facing; update it in the same commit when a control changes
- `docs/companion.md` — grid, button JSON, expressions, internal actions, HTTP/tRPC APIs, debugging, the module fork
- `docs/macos.md` — `mediakey` and `zored_stream` volume + Now Playing (perl child), permissions, LaunchAgent
- `docs/vkcall.md` — VK WorkSpace call mic/camera: the client's gRPC API, call module of `zored_stream`, buttons
- `docs/ai.md` — Claude usage limits: `usage` module of `zored_stream`, right strip, Hammerspoon overlay
- `docs/ymusic.md` — Yandex Music like/dislike via Accessibility, `zored_stream` module, dead ends
- `docs/windows.md` — port plan: what carries over, what needs a Windows build
- `docs/device.md` — USB, raw protocol events, `tools/sniff.py`, firmware

## Loop

1. Edit pages in `tools/gen_pages.py` (the generated `companion/*` files are build output, committed).
2. `make deploy` — regenerates pages, creates custom variables, imports pages into the running Companion. Overwrites pages 1 (MEDIA) and 2 (TEST).
3. Verify before reporting done:
   - inputs: `custom_log` actions or debug log lines `Button <page>/<row>/<col> pressed`
   - state: `GET /api/custom-variable/<name>/value`, `POST /api/location/<p>/<r>/<c>/press`
   - looks: web preview at `http://127.0.0.1:8000/buttons/1`; strips and haptics only on the device — ask the user

Helpers: `make mediakey`, `make zored_stream` (LaunchAgent), `make module` (builds the module fork, restarts Companion), `make companion` (autostart). Auto mode blocks all but `mediakey`: hand them to the user.

## Rules

- Parallel sessions: only one at a time edits `tools/gen_pages.py` or runs `make deploy`; claim controls from the free map in `PLAN.md`.
- Commit only your files: `git -C <repo> commit -m main -- <paths>`.

- One process owns the serial port: Companion, Loupedeck or `sniff.py`.
- Firmware: flash or send update commands only after explicit confirmation in chat (bricking risk). Vendor binaries stay out of git.
- Mark findings verified (observed) or assumed (read).
- Device style is minimalist: no labels or units (`%`), dots over words, lowercase text, white values, gray (#808080) secondary,
  color only for warnings or a transient state; 6% inset. Offer the sparest variant first.
- Secrets stay out of the repo: the Companion REST key lives in `~/.config/zored-stream/companion-token`.
- `gh`: run as `env -u GITHUB_TOKEN gh …` (the env token is stale, the keyring login works).
