# Razer Stream Controller

A rebranded Loupedeck Live: 12 touch tiles, 6 knobs (turn + press), 8 RGB buttons, two touch strips.

## USB and protocol

- VID `0x1532`, PID `0x0D06` (Stream Controller X: `0x0D09`). Interfaces: CDC serial, vendor-specific, audio. **No HID.**
- Serial 8N1, WebSocket upgrade over the serial line, then binary frames. Full notes:
  [ShoelessTim/RazerStream PROTOCOL.md](https://github.com/ShoelessTim/RazerStream/blob/main/PROTOCOL.md).
- Device → host events (verified with `tools/sniff.py`): press `0x00 <id> 00|01` (down/up), rotate `0x01 <id> ±1`,
  touch move/end `0x4d`/`0x6d`. IDs: knobs left `0x01..0x03`, right `0x04..0x06`; buttons `0x07..0x0e`.
- Haptics: command `0x1B` (`SetVibration`) with one pattern byte.
- Firmware 0.2.14 echoes the handshake back as `0x73` frames; drivers log them as unhandled, harmless.

`tools/sniff.py /dev/cu.usbmodemXXXX <seconds>` prints raw events. Close Companion and Loupedeck first.

## Touch

One touch surface across strips and tiles (verified, sniff 2026-11-03):

- `0x4d`/`0x6d` payload: `?? xH xL yH yL id`; raw x 0..480 (left strip <60, center 60..420, right ≥420), y 0..270, tile ≈90 px.
- No touchstart: the first `0x4d` of an id is the start (`@loupedeck/core` emits `touchstart`). A tap is one `0x4d` + `0x6d`.
- Move events every 10–20 ms (~70/s) while the finger moves, none while it rests; pixel resolution.
- A drag across tiles or onto a strip stays one touch id, so tile swipes are possible.

The module fork ignores center `touchmove` and resolves `touch.target.control` per event: dragging tile A → B sends
keyDown A, keyUp B (A sticks); releasing in a gap sends no keyUp. A tile swipe needs the control pinned at touchstart,
a direction check at touchend (~40 px), one POST to the daemon per swipe, and a rule for tap/hold conflicts.

## Firmware

0.2.14 (Loupedeck 6.4.1 reports "up to date"). Custom firmware is parked:

- nothing public: no custom firmware, dump or MCU teardown; GitHub has only host-side drivers
- the vendor image (`FW_C40_T0_0.2.14.bin`, downloaded by Loupedeck) has entropy 8.0 bits/byte, no vector table, no strings → encrypted
- the remaining path is opening the device and trying SWD; likely read-protected, bricking risk

Never flash or send firmware-update commands without explicit confirmation.

## Official Loupedeck software (reference)

Loupedeck 6.4.1 on macOS = Electron UI + `LogiPluginService` (owns the device, autostarts via `com.logi.pluginservice.launch`).
Data in `~/Library/Application Support/Logi/LogiPluginService/`; the RSC is device type `Loupedeck40`; profiles in
`Applications/Loupedeck40/<app>/Profiles/<GUID>/ProfileInfo.json`.
