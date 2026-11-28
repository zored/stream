$ErrorActionPreference = 'Stop'

Add-Type -Path (Join-Path $PSScriptRoot 'sense-battery.cs')
[void][Windows.Gaming.Input.RawGameController, Windows.Gaming.Input, ContentType=WindowsRuntime]

$levels = [ordered]@{
    xbox = $null
    sense_left = $null
    sense_right = $null
}

foreach ($controller in [Windows.Gaming.Input.RawGameController]::RawGameControllers) {
    if ($controller.HardwareVendorId -ne 0x045e) { continue }
    if ($controller.HardwareProductId -ne 0x02e0 -and $controller.DisplayName -notlike 'Xbox*') { continue }
    $battery = $controller.TryGetBatteryReport()
    if ($null -eq $battery -or $battery.Status -in 'NotPresent', 'Unknown') { continue }
    $remaining = $battery.RemainingCapacityInMilliwattHours
    $full = $battery.FullChargeCapacityInMilliwattHours
    if ($null -ne $remaining -and $full -gt 0 -and $remaining -ge 0 -and $remaining -le $full) {
        $levels.xbox = [int][Math]::Round(100 * $remaining / $full)
        break
    }
}

$sense = [SenseBattery]::Read()
if ($sense.ContainsKey(0x0e45)) { $levels.sense_left = $sense[0x0e45] }
if ($sense.ContainsKey(0x0e46)) { $levels.sense_right = $sense[0x0e46] }
ConvertTo-Json -Compress -InputObject $levels
