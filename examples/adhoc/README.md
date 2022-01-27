## Adhoc mode examples

These examples showcase the different running modes of `pyroscope adhoc`.
Check [adhoc documentation](https://pyroscope.io/docs/agent-configuration-adhoc) for complete information about what modes are supported by each language.

Note: The example programs are toy examples but they share some interesting properties: their running time should take around 1 minute, and their profiling data should change slightly from run to run (making them good examples for comparison / diff mode visualizations).

### Push Mode
If the application to profile is already using an agent or has some integration with the HTTP API,
push mode can be used to profile the application without any configuration change.

#### Golang adhoc (push)
```
# no spy is supported, --push is not needed (but can still be provided):
pyroscope adhoc go run adhoc-push.go
```
#### Python adhoc (using pip package)
```
# pyspy is autodetected, --push is mandatory.
# Note that you need pyroscope-io >= 0.6.0 for this to work.
pyroscope adhoc --push python adhoc-push.py
```


### Exec mode
For languages with a supported spy and no other pyroscope integration, this is the easiest way to get profiling data.
For example, this method will work for python or ruby _without_ the pip/gem instrumented in the code.

```
# Run with spy-name autodetected.
pyroscope adhoc python adhoc-spy.py

# Alternatively, specify the spy-name if it cannot be autodetected.
pyroscope adhoc --spy-name pyspy ./adhoc-spy.py
```

### Connect mode
#### Profile a process that is already running

If the profiled process is already running, it's possible to attach to it instead, indicating its PID through the `--pid` flag:

```
# Run the program normally, in the background. It should give the PID as output.
python adhoc-spy.py &
# => [1] 841690
# Use adhoc to attach to the running command.
# Note that the --spy-name is now mandatory as it cannot be inferred.
# Also, pyroscope needs to be launched as root to be able to trace the running process.
sudo pyroscope adhoc --spy-name pyspy --pid 841690
```

### Pull mode

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
