# CLAUDE.md

Config-as-code for a Razer Stream Controller (= Loupedeck Live) driven by Bitfocus Companion, on macOS now, Windows next.
Status and next steps: `PLAN.md`.

## Docs — read before touching the area

- `docs/companion.md` — grid, button JSON, expressions, internal actions, HTTP/tRPC APIs, debugging, the module patch
- `docs/macos.md` — `mediakey` and `streamd` (volume, Now Playing via perl), permissions, LaunchAgent
- `docs/vkcall.md` — VK WorkSpace call mic/camera: the client's gRPC API, `vkcall` daemon, buttons
- `docs/ymusic.md` — Yandex Music like/dislike via Accessibility, `ymusic` daemon, dead ends
- `docs/windows.md` — port plan: what carries over, what needs a Windows build
- `docs/device.md` — USB, raw protocol events, `tools/sniff.py`, firmware

## Loop

1. Edit pages in `tools/gen_pages.py` (the generated `companion/*` files are build output, committed).
2. `make deploy` — regenerates pages, creates custom variables, imports pages into the running Companion. Overwrites pages 1 (MEDIA) and 2 (TEST).
3. Verify before reporting done:
   - inputs: `custom_log` actions or debug log lines `Button <page>/<row>/<col> pressed`
   - state: `GET /api/custom-variable/<name>/value`, `POST /api/location/<p>/<r>/<c>/press`
   - looks: web preview at `http://127.0.0.1:8000/buttons/1`; strips and haptics only on the device — ask the user

Helpers: `make mediakey`, `make streamd`, `make vkcall`, `make ymusic` (LaunchAgents), `make patch-module` (+ Companion restart). Auto mode blocks all but `mediakey`: hand them to the user.

## Rules

- Parallel sessions: only one at a time edits `tools/gen_pages.py` or runs `make deploy`; claim controls from the free map in `PLAN.md`.
- Commit only your files: `git -C <repo> commit -m main -- <paths>`.

- One process owns the serial port: Companion, Loupedeck or `sniff.py`.
- Firmware: flash or send update commands only after explicit confirmation in chat (bricking risk). Vendor binaries stay out of git.
- Mark findings verified (observed) or assumed (read).
- Secrets stay out of the repo: the Companion REST key lives in `~/.config/zored-stream/companion-token`.
- `gh`: run as `env -u GITHUB_TOKEN gh …` (the env token is stale, the keyring login works).
