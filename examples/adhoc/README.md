## Adhoc mode examples

Pyroscope provides three options for using the "adhoc" mode, depending on whether you have the Pyroscope SDK installed or not. Choose the appropriate method based on your setup.

### Option 1: Push mode (with SDK installed)
If your application already uses an agent or has integration with the Pyroscope HTTP API, you can use push mode
to profile the application without any additional configuration changes.

#### Golang Adhoc (Push Mode)
If the application to profile is already using an agent or has some integration with the HTTP API,
push mode can be used to profile the application without any configuration change.

```
# no spy is supported, --push is not needed (but can still be provided):
pyroscope adhoc go run adhoc-push.go
```

#### Python adhoc (using pip package)
```
# pyspy is autodetected, --push is mandatory.
pyroscope adhoc --push python adhoc-push.py
```

### Option 2: Push Mode (no SDK installed -- Pyroscope sidecar)

If you don't have the Pyroscope SDK installed and want to profile a Python or Ruby application, you can still push data
to the Pyroscope server using the adhoc mode as a sidecar.

#### Python adhoc (no pip package -- Pyroscope sidecar)
For languages with a supported spy and no other pyroscope integration, this is the easiest way to get profiling data.
For example, this method will work for python or ruby _without_ the pip/gem instrumented in the code.
For this you will need to use the `pyroscope-cli` from the [pyroscope-rs](https://github.com/grafana/pyroscope-rs/releases/tag/cli-0.2.7) project.

**Note: this is different than using the pyroscope binary, `./pyroscope`, itself.**

Profile a script using adhoc
```
pyroscope adhoc --push pyroscope-cli exec --spy-name=pyspy main.py
```

Attach to a process and profile it using `pyroscope-cli`
```
sudo pyroscope adhoc --push pyroscope-cli connect --spy-name=pyspy --pid=<pid>
```

```
```

### Option 3: Pull Mode

If the application to profile supports pull-mode, that is, it's already running a HTTP server
and serving profiling data in a supported format, like `pprof`,
pull-mode can be used to to profile the application without any configuration change,
as long as the HTTP endpoint is reachable for pyroscope.

In this mode, pyroscope can either launch the application if needed or just gather the data of an already running application:

```
# Launch and profile the application
pyroscope adhoc --url localhost:6060 go run adhoc-pull.go
```

```
# Lauch the application first
go run adhoc-pull.go &
# Profile the running application.
# Note that currently pyroscope will be running until you interrupt it.
pyroscope adhoc --url localhost:6060
```
