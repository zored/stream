# Battery levels on Windows

Pair **Ulanzi Dial** in Windows Bluetooth settings, then build `dialctl.exe` from [d100h](https://github.com/zored/d100h) with `go build ./cmd/dialctl`. With Node.js 22 or newer on PATH, install the refresh task from this repository:

```powershell
.\tools\windows\install-battery-levels.ps1 -Dialctl C:\path\to\dialctl.exe
```

Keep the executable at that path. The task runs under the signed-in user's account once a minute. It replaces the keyboard flag artwork on the Windows MEDIA tile at page 1, row 0, column 4 with the dial's charge. If the reading fails, the tile displays a dash. The task reapplies the artwork after a page deployment when the flag layer is present.

`%USERPROFILE%\.config\zored-stream\battery-levels.json` contains a map from device IDs to integer percentages, for example `{"d100h":100}`. An unavailable device is absent from the map. The refresh task records errors in `battery-levels.log` in the same directory.

Add device readers to `sources` in [battery-levels.mjs](battery-levels.mjs). Candidates observed in Windows are the paired Xbox Wireless Controller and the PlayStation VR2 Sense left and right controllers. Their battery interfaces still need verification. For other devices, check BLE Battery Service, HID and Windows PnP/WMI battery properties. On macOS, use the Razer battery reader when available.
