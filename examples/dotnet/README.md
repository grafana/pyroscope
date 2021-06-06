# .NET Examples

The directory contains examples of how to run Pyroscop to profile .NET applications in Docker containers.

### Fast-slow

The example is a simple single-thread application similar to examples for other spies.

The code is pretty self-explanatory: `Slow.Work` should take 80% of CPU time and remaining 20% to be consumed by
`Fast.Work`. You may ask why `Fast` and `Slow` classes are defined in separate namespaces. The fact is that Pyroscope
colors frames depending on the namespaces (for .NET traces), so they are defined in this way just for sake of demo ;)

In this example, the program is built as an executable (`/dotnet/example`), therefore `-spy-name dotnetspy` argument is
required to tell Pyroscope how to deal with the process:
> CMD ["pyroscope", "exec", "-spy-name", "dotnetspy", "/dotnet/example"]

To run the example execute the following commands:

```shell
# cd examples/dotnet/fast-slow
# docker-compose up
```

### Web

The example is a simple ASP.NET Core web app that enables you to observe how consumed CPU time is changed by making
HTTP requests to [http://localhost:5000](http://localhost:5000).

In contrast to the previous example, the application is built as a dynamic library (`/dotnet/example.dll`) and
runs with dotnet. This allows Pyroscope to automatically detect which kind of spy to use, making `-spy-name` option
unnecessary:
> CMD ["pyroscope", "exec", "dotnet", "/dotnet/example.dll"]

To run the example execute the following commands:

```shell
# cd examples/dotnet/web
# docker-compose up
```
