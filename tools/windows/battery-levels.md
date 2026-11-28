# Battery levels on Windows

Pair **Ulanzi Dial** in Windows Bluetooth settings, then build `dialctl.exe` from [d100h](https://github.com/zored/d100h) with `go build ./cmd/dialctl`. With Node.js 22 or newer on PATH, install the refresh task from this repository:

```powershell
.\tools\windows\install-battery-levels.ps1 -Dialctl C:\path\to\dialctl.exe
```

Keep the executable at that path. The task runs under the signed-in user's account once a minute. The Windows MEDIA tile at page 1, row 0, column 4 shows the known readings in place of the keyboard flag artwork. One reading fills the tile, and multiple readings use a compact grid. The task reapplies the artwork after a page deployment when the flag layer is present.

`%USERPROFILE%\.config\zored-stream\battery-levels.json` maps device IDs to integer percentages, for example `{"d100h":100,"xbox":null,"sense_left":null,"sense_right":null}`. Devices without a reading have a JSON value of `null` and are hidden from the tile. The refresh task records unexpected errors in `battery-levels.log` in the same directory.

- D100H uses its vendor BLE GATT service.
- Xbox Wireless Controller uses [Windows Gaming Input](https://learn.microsoft.com/en-us/uwp/api/windows.gaming.input.igamecontrollerbatteryinfo.trygetbatteryreport) to read the remaining and full charge capacities.
- PlayStation VR2 Sense L/R use [Bluetooth HID report `0x31`](https://github.com/ctrl-shift-win-b/PSVR2-Documentation/blob/main/PSVR2_DATASHEET_COMPACT.md#102-input-report-0x31-controller--host-78-b). Reading calibration feature `0x05` starts the extended input reports. The battery level changes in 10-point steps.

For other devices, add a reader in [battery-levels.mjs](battery-levels.mjs). Sources to check include BLE Battery Service, HID and Windows PnP/WMI battery properties. On macOS, use the Razer battery reader when available.
