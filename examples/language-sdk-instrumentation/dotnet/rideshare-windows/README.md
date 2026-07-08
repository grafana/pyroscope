# .NET rideshare example (Windows)

The same rideshare app as the [`rideshare`](../rideshare) example, but running
in **Windows containers** under the Pyroscope .NET profiler. Windows support
shipped in pyroscope-dotnet 1.3.0.

The profiler on Windows is a single native DLL (`Pyroscope.Profiler.Native.dll`)
attached through the standard CoreCLR profiling environment variables — there is
no `LD_PRELOAD`/ApiWrapper as on Linux.

## How it is put together

A Docker engine runs either Linux containers or Windows containers, never both,
and Pyroscope and Grafana only ship Linux images. So the example is split into
two compose files:

- `docker-compose.server.yml` — Pyroscope + Grafana, on the **Linux** engine
  (Docker Desktop in Linux-containers mode, or Docker inside WSL2), with ports
  `4040`/`3000` published on the host.
- `docker-compose.yml` — the rideshare regions + load generator as **Windows**
  containers, uploading profiles to the host-published Pyroscope port.

## Run it

Two supported setups, differing only in where the two Docker engines come from.
Either way, the first build pulls Windows base images, which are considerably
larger than Linux ones — expect the first `docker compose up --build` to take a
while.

### With Docker Desktop

The defaults are tuned for Docker Desktop: images use `ltsc2022` bases (which
run on any host under the default Hyper-V isolation) and the apps upload to
`host.docker.internal`, which Docker Desktop provides.

1. In **Linux containers** mode, start the server half:

   ```powershell
   docker compose -f docker-compose.server.yml up -d
   ```

2. Switch Docker Desktop to **Windows containers** (system tray → "Switch to
   Windows containers..."). The Linux containers keep running.

3. Start the app half:

   ```powershell
   docker compose up --build
   ```

4. Open Grafana at http://localhost:3000, go to Drilldown → Profiles, and look
   for `rideshare.dotnet.windows.app`.

### On Windows Server, no Docker Desktop

Verified on Windows Server 2025 with the [static docker-ce
engine](https://learn.microsoft.com/en-us/virtualization/windowscontainers/quick-start/set-up-environment)
for Windows containers and Docker inside a WSL2 distro for the Linux half.
Three things differ from Docker Desktop: process isolation needs
host-matching image bases, `host.docker.internal` does not exist, and ports
published inside WSL need a forward to be reachable from Windows containers.

Run everything as the user that registered the WSL distro — from another
account (for example an SSM session, which runs as SYSTEM) `wsl` fails with
"There is no distribution with the supplied name".

First, keep WSL alive for the whole demo: WSL shuts down seconds after its
last client exits, taking the Linux containers with it. Open a separate
terminal, run `wsl -d Ubuntu` (your distro name — see `wsl --list`), and leave
it open.

Then, from this directory in an elevated PowerShell:

```powershell
# the server half, on the Linux engine inside WSL
$wslRepo = ((Get-Location).Path -replace '^C:', '/mnt/c') -replace '\\', '/'
wsl -d Ubuntu -u root -- docker compose -f $wslRepo/docker-compose.server.yml up -d

# allow inbound 4040 (once)
New-NetFirewallRule -DisplayName pyroscope-4040 -Direction Inbound -Protocol TCP -LocalPort 4040 -Action Allow

# forward host:4040 to WSL, where the Linux engine published it
# (the WSL IP changes when WSL restarts - re-run these two lines after one)
$wslIp = (wsl -d Ubuntu -u root -- hostname -I).Trim().Split(' ')[0]
netsh interface portproxy add v4tov4 listenport=4040 listenaddress=0.0.0.0 connectaddress=$wslIp connectport=4040

# the app half, on the Windows engine:
# - under process isolation the image base must match the host (Server 2025 below);
#   the plain multi-arch dotnet tags do not even resolve there, hence explicit tags
# - containers reach the host by its primary IP; note that the gateway IP of a
#   *different* Docker network does not route from the compose network
$env:WINDOWS_BASE_TAG = "nanoserver-ltsc2025"
$hostIp = (Get-NetIPConfiguration | Where-Object { $_.IPv4DefaultGateway }).IPv4Address.IPAddress | Select-Object -First 1
$env:PYROSCOPE_SERVER_ADDRESS = "http://${hostIp}:4040"
docker compose up -d --build
```

Verify profiles are arriving (Pyroscope takes ~45 s to report ready):

```powershell
curl.exe -s -X POST -H 'Content-Type: application/json' -d '{"name":"service_name"}' http://localhost:4040/querier.v1.QuerierService/LabelValues
# {"names":["pyroscope","rideshare.dotnet.windows.app"]}
```

Grafana is reachable at `http://<wsl-ip>:3000` (or forward port 3000 the same
way as 4040).

## Run natively (no containers)

The app also runs directly on the host under the profiler — closer to how a
Windows service or IIS deployment would look:

```powershell
.\run.ps1     # downloads the released profiler, publishes, attaches, runs
.\load.ps1    # in a second shell: generate load
```

`run.ps1` defaults to `http://localhost:4040`; to send to Grafana Cloud
Profiles instead:

```powershell
.\run.ps1 -ServerAddress https://profiles-prod-XXX.grafana.net `
          -BasicAuthUser <instanceID> -BasicAuthPassword <token>
```

TLS uploads work out of the box — the profiler verifies the server certificate
against the Windows system certificate store. For a private/internal CA, point
`SSL_CERT_FILE` at a PEM bundle before running.

## Endpoints

- `/bike`, `/scooter`, `/car` — order a vehicle (the profiled work).
- `/playground/{allocation,contention,exception,leak}` — exercise specific
  profile types.
- `/pyroscope/{cpu,allocation,contention,exception}?enable=true|false` — toggle
  a profile type at runtime via the managed API.

## Notes

- The profiler is attached with:

  ```
  CORECLR_ENABLE_PROFILING=1
  CORECLR_PROFILER={BD1A650D-AC5D-4896-B64F-D6FA25D6B26A}
  CORECLR_PROFILER_PATH=<...>\Pyroscope.Profiler.Native.dll
  PYROSCOPE_SERVER_ADDRESS=<server>
  PYROSCOPE_PROFILING_ENABLED=1
  PYROSCOPE_PROFILING_{ALLOCATION,CONTENTION,EXCEPTION,HEAP}_ENABLED=true
  ```

- CoreCLR / modern .NET (8/9/10): CPU + allocation + contention + exception.
  .NET Framework 4.8 is CPU-only (allocation needs ETW infrastructure that this
  build does not ship).
- Unlike the Linux example, no OTLP trace exporter is wired up (there is no
  collector here); the Pyroscope span processor still tags profiles with span
  context. Add an exporter in `example/Program.cs` if you want traces too.
