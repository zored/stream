# CLAUDE.md

Repo about stream controllers: configs, protocol notes, custom host software and (maybe) firmware.
Current device: Razer Stream Controller (= Loupedeck Live hardware). Roadmap in `PLAN.md`.

## Facts

- USB VID `0x1532`, PID `0x0D06`; serial CDC, WebSocket upgrade, binary frames. Details: `PROTOCOL.md` in ShoelessTim/RazerStream.
- Official app (Loupedeck) holds the USB port: quit it before talking to the device from anything else.
- Existing tools are host-side drivers, not firmware. Don't call them firmware.
- Loupedeck 6.4.1 on macOS = Electron UI `/Applications/Loupedeck.app` + `/Applications/Utilities/LogiPluginService.app` (the service owns the device).
- Service data: `~/Library/Application Support/Logi/LogiPluginService/`. RSC is device type `Loupedeck40`.
  - Profiles: `Applications/Loupedeck40/<app>/Profiles/<GUID>/ProfileInfo.json` (pages, controls, actions) + `ActionImages/`, `ActionIcons/`, `ActionColors/`.
  - Settings: `LoupedeckSettings.ini` (don't edit while the service runs). Auto backups: `Applications.Backups/*.zip`.
  - Firmware downloads: `SotfwareUpdates/FW_C40_T0_<ver>.bin` (vendor blob, research only, never commit).

## Local references

- `~/git/github.com/SoelessTim/RazerStream` — clone of ShoelessTim/RazerStream (Swift, macOS 14+, SwiftPM + Makefile). Folder name has a typo, the GitHub owner is `ShoelessTim`.

## Rules

- Never flash firmware or send firmware-update commands to the device without explicit confirmation in chat: bricking risk.
- Don't commit vendor binaries (firmware dumps, Loupedeck installers, plugins). Link them instead.
- Mark findings as verified (tried on the device) or assumed (read somewhere).
- Builds and knowledge sharing go through GitHub Actions: anything that only builds on one laptop isn't done.
- `gh`: `GITHUB_TOKEN` in the env may be stale; the keyring login works with `env -u GITHUB_TOKEN gh …`.
