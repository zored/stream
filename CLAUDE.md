# CLAUDE.md

Repo about stream controllers: configs, protocol notes, custom host software and (maybe) firmware.
Current device: Razer Stream Controller (= Loupedeck Live hardware). Roadmap in `PLAN.md`.

## Facts

- USB VID `0x1532`, PID `0x0D06`; serial CDC, WebSocket upgrade, binary frames. Details: `PROTOCOL.md` in ShoelessTim/RazerStream.
- Official app (Loupedeck) holds the USB port: quit it before talking to the device from anything else.
- Existing tools are host-side drivers, not firmware. Don't call them firmware.

## Local references

- `~/git/github.com/SoelessTim/RazerStream` — clone of ShoelessTim/RazerStream (Swift, macOS 14+, SwiftPM + Makefile). Folder name has a typo, the GitHub owner is `ShoelessTim`.

## Rules

- Never flash firmware or send firmware-update commands to the device without explicit confirmation in chat: bricking risk.
- Don't commit vendor binaries (firmware dumps, Loupedeck installers, plugins). Link them instead.
- Mark findings as verified (tried on the device) or assumed (read somewhere).
- Builds and knowledge sharing go through GitHub Actions: anything that only builds on one laptop isn't done.
- `gh`: `GITHUB_TOKEN` in the env may be stale; the keyring login works with `env -u GITHUB_TOKEN gh …`.
