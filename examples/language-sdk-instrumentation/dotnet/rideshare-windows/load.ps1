# Drives the rideshare endpoints in a loop so the profiler has something to see.
# Run in a second shell while run.ps1 is running.
[CmdletBinding()]
param(
    [int]$Port = 5000,
    [int]$DurationSeconds = 0   # 0 = run until Ctrl+C
)
$ErrorActionPreference = "Continue"
$ProgressPreference = "SilentlyContinue"
$endpoints = @("bike", "scooter", "car", "playground/allocation", "playground/contention", "playground/exception")
$deadline = if ($DurationSeconds -gt 0) { (Get-Date).AddSeconds($DurationSeconds) } else { [DateTime]::MaxValue }
Write-Host ">> driving http://localhost:$Port  (Ctrl+C to stop)"
while ((Get-Date) -lt $deadline) {
    foreach ($ep in $endpoints) {
        try { Invoke-WebRequest -UseBasicParsing -TimeoutSec 30 "http://localhost:$Port/$ep" | Out-Null } catch {}
    }
    Start-Sleep -Milliseconds 300
}
