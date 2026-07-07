# Runs the rideshare example on Windows under the Pyroscope .NET profiler.
#
# It downloads the released Windows profiler (a single native DLL), publishes
# the app, attaches the profiler via the CORECLR_* environment variables, and
# starts it. Point it at a local Pyroscope (default) or at Grafana Cloud.
#
#   .\run.ps1                                        # -> http://localhost:4040
#   .\run.ps1 -ServerAddress https://<stack>.grafana.net `
#             -BasicAuthUser <instanceId> -BasicAuthPassword <token>
[CmdletBinding()]
param(
    [string]$Version = "1.3.0",
    [string]$ServerAddress = "http://localhost:4040",
    [string]$ApplicationName = "rideshare.dotnet.windows.app",
    [string]$Region = "us-east",
    [string]$BasicAuthUser = "",
    [string]$BasicAuthPassword = "",
    [string]$Framework = "net8.0",
    [int]$Port = 5000
)
$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"  # much faster Invoke-WebRequest
$root = $PSScriptRoot

# 1. Fetch the Windows profiler from the pyroscope-dotnet release (cached).
$profilerDir = Join-Path $root ".profiler\$Version"
$profilerDll = Join-Path $profilerDir "Pyroscope.Profiler.Native.dll"
if (-not (Test-Path $profilerDll)) {
    New-Item -ItemType Directory -Force -Path $profilerDir | Out-Null
    $zip = Join-Path $profilerDir "pyroscope-windows-x64.zip"
    $url = "https://github.com/grafana/pyroscope-dotnet/releases/download/pyroscope-$Version/pyroscope.$Version-windows-x64.zip"
    Write-Host ">> Downloading profiler $Version from $url"
    Invoke-WebRequest -Uri $url -OutFile $zip
    Expand-Archive -Path $zip -DestinationPath $profilerDir -Force
    if (-not (Test-Path $profilerDll)) { throw "Pyroscope.Profiler.Native.dll not found in the release archive" }
}

# 2. Publish the app.
$publishDir = Join-Path $root "bin\publish\$Framework"
Write-Host ">> Publishing rideshare ($Framework)"
dotnet publish (Join-Path $root "example") -c Release --framework $Framework --runtime win-x64 --no-self-contained -o $publishDir
if ($LASTEXITCODE -ne 0) { throw "dotnet publish failed" }

# 3. Attach the profiler and run.
$env:CORECLR_ENABLE_PROFILING = "1"
$env:CORECLR_PROFILER = "{BD1A650D-AC5D-4896-B64F-D6FA25D6B26A}"
$env:CORECLR_PROFILER_PATH = $profilerDll
$env:PYROSCOPE_APPLICATION_NAME = $ApplicationName
$env:PYROSCOPE_SERVER_ADDRESS = $ServerAddress
$env:PYROSCOPE_LABELS = "region:$Region"
$env:PYROSCOPE_PROFILING_ENABLED = "1"
$env:PYROSCOPE_PROFILING_ALLOCATION_ENABLED = "true"
$env:PYROSCOPE_PROFILING_CONTENTION_ENABLED = "true"
$env:PYROSCOPE_PROFILING_EXCEPTION_ENABLED = "true"
$env:PYROSCOPE_PROFILING_HEAP_ENABLED = "true"
$env:PYROSCOPE_LOG_LEVEL = "debug"
if ($BasicAuthUser)     { $env:PYROSCOPE_BASIC_AUTH_USER = $BasicAuthUser }
if ($BasicAuthPassword) { $env:PYROSCOPE_BASIC_AUTH_PASSWORD = $BasicAuthPassword }
$env:ASPNETCORE_URLS = "http://localhost:$Port"

Write-Host ""
Write-Host ">> rideshare listening on http://localhost:$Port"
Write-Host ">> endpoints: /bike /scooter /car /playground/{allocation,contention,exception,leak}"
Write-Host ">> uploading '$ApplicationName' to $ServerAddress  (Ctrl+C to stop)"
Write-Host ">> generate load in another shell:  .\load.ps1 -Port $Port"
Write-Host ""
& (Join-Path $publishDir "example.exe")
