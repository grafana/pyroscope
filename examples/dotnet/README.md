# .NET Examples

The directory contains examples of how to run Pyroscope to profile .NET applications in Docker containers.

### Fast-slow

The example is a simple single-thread application similar to examples for other spies.

The code is pretty self-explanatory: `Slow.Work` should take 80% of CPU time and remaining 20% to be consumed by
`Fast.Work`. You may ask why `Fast` and `Slow` classes are defined in separate namespaces. The fact is that Pyroscope
colors frames depending on the namespaces (for .NET traces), so they are defined in this way just for sake of demo ;)

To run the example execute the following commands:

```shell
# cd examples/dotnet/fast-slow
# docker-compose up
```
