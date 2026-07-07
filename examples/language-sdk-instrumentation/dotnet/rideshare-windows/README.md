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

1. Start the server half on the Linux engine:

   ```powershell
   docker compose -f docker-compose.server.yml up -d
   ```

2. Switch Docker Desktop to **Windows containers** (system tray → "Switch to
   Windows containers..."). The Linux containers keep running.

3. Start the app half on the Windows engine:

   ```powershell
   docker compose up --build
   ```

4. Open Grafana at http://localhost:3000, go to Drilldown → Profiles, and look
   for `rideshare.dotnet.windows.app`.

The first build pulls Windows base images, which are considerably larger than
Linux ones — expect the first `docker compose up --build` to take a while.

### Windows base image version

The images default to `ltsc2022` bases, which run everywhere under Docker
Desktop (Hyper-V isolation). On a Windows Server host with process isolation
the base must match the host version — the plain multi-arch dotnet tags do not
even resolve there. On Server 2025:

```powershell
$env:WINDOWS_BASE_TAG = "nanoserver-ltsc2025"; docker compose up --build
```

### If `host.docker.internal` does not resolve

The app containers default to `http://host.docker.internal:4040`, which Docker
Desktop provides. On a plain Windows Server docker engine (no Docker Desktop),
point them at the host's primary IP instead — note that the gateway IP of a
*different* Docker network won't route from the compose network:

```powershell
$env:PYROSCOPE_SERVER_ADDRESS = "http://<host-primary-ip>:4040"; docker compose up --build
```

And if the Linux half runs inside WSL2 (rather than Docker Desktop), its
published ports are only visible inside WSL — forward the host port to it:

```powershell
netsh interface portproxy add v4tov4 listenport=4040 listenaddress=0.0.0.0 connectaddress=<wsl-ip> connectport=4040
New-NetFirewallRule -DisplayName pyroscope-4040 -Direction Inbound -Protocol TCP -LocalPort 4040 -Action Allow
```

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
