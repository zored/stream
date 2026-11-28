param(
    [Parameter(Mandatory = $true)]
    [string]$Dialctl
)

$ErrorActionPreference = 'Stop'
$taskName = 'zored-stream-battery'
$batteryReader = (Resolve-Path -LiteralPath $Dialctl).Path
$publisher = Join-Path $PSScriptRoot 'battery-levels.mjs'
$state = Join-Path $env:USERPROFILE '.config/zored-stream'
$output = Join-Path $state 'battery-levels.json'
$log = Join-Path $state 'battery-levels.log'
$node = (Get-Command node.exe -CommandType Application | Select-Object -First 1).Source
$powershell = (Get-Command powershell.exe -CommandType Application | Select-Object -First 1).Source
$user = [Security.Principal.WindowsIdentity]::GetCurrent().Name

New-Item -ItemType Directory -Path $state -Force | Out-Null
function Quote-Literal([string]$Value) {
    return "'" + $Value.Replace("'", "''") + "'"
}
$command = "& $(Quote-Literal $node) $(Quote-Literal $publisher) $(Quote-Literal $batteryReader) $(Quote-Literal $output) 2> $(Quote-Literal $log); exit " + '$LASTEXITCODE'
$encoded = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($command))
$action = New-ScheduledTaskAction -Execute $powershell -Argument "-NoProfile -NonInteractive -WindowStyle Hidden -EncodedCommand $encoded"
$trigger = New-ScheduledTaskTrigger -Once -At (Get-Date).AddMinutes(1) -RepetitionInterval (New-TimeSpan -Minutes 1)
$principal = New-ScheduledTaskPrincipal -UserId $user -LogonType Interactive -RunLevel Limited
$settings = New-ScheduledTaskSettingsSet -ExecutionTimeLimit (New-TimeSpan -Seconds 55) -MultipleInstances IgnoreNew -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries
Register-ScheduledTask -TaskName $taskName -Action $action -Trigger $trigger -Principal $principal -Settings $settings -Description 'Refresh battery levels JSON and the Companion D100H tile once a minute.' -Force | Out-Null
Start-ScheduledTask -TaskName $taskName
Write-Output $output
