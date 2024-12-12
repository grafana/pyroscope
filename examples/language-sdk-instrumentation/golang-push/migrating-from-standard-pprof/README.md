# Migrating from standard pprof to Pyroscope in a Go application

This README provides a comprehensive guide on migrating from the standard `pprof` library to Pyroscope in a Go application. The example demonstrates the transition within a detective-themed Go application, showcasing how to enhance the profiling process with Pyroscope's advanced capabilities. The actual changes needed to migrate from the standard `pprof` to using [the Pyroscope SDK](https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/go_push/) are very simple. See [the source PR](https://github.com/grafana/pyroscope/pull/2830).

The Pyroscope Go SDK extends the standard `pprof` library with extra functionality and performance improvements. If you would like to use the standard `pprof` _alongside_ the Pyroscope Go SDK simultaneously, see the [example here](https://github.com/grafana/pyroscope-go/tree/main/example/http). 

<img width="1426" alt="image" src="https://github.com/grafana/pyroscope/assets/23323466/f094399a-4a4d-4b47-9f03-5a15b4085fab">

## Changes made

### Pre-Pyroscope setup

Originally in the pre-pyroscope code, the `main.go` file used the standard `net/http/pprof` package for profiling. This setup is common and straightforward but lacks continuous profiling and real-time analysis capabilities.

### Post-Pyroscope migration

In the post-pyroscope code, to leverage the advanced features of Pyroscope, we made the following changes:

1. **Removed Standard pprof Import:** The `_ "net/http/pprof"` import was removed, as Pyroscope replaces its functionality.


2. **Added Pyroscope SDK:** We installed the Pyroscope module using the following command and imported it in our `main.go`:

> go get github.com/grafana/pyroscope-go

3. **Enabled block and mutex profilers:** This step is only required if you're using mutex or block profiling. Inside the `main()` function, we enabled the block and mutex profilers using the runtime functions: 

```go
runtime.SetMutexProfileFraction(5)
runtime.SetBlockProfileRate(5)
```

4. **Configured Pyroscope:** Inside the `main()` function, we set up Pyroscope using the `pyroscope.Start()` method with the following configuration:
   - Application name and server address.
   - Logger configuration.
   - Tags for additional metadata.
   - Profile types to be captured.


5. **Consider using [godeltaprof](https://pkg.go.dev/github.com/grafana/pyroscope-go/godeltaprof):** It provides an optimized way to perform memory, mutex, and block profiling more efficiently.

## Benefits of using Pyroscope

- **Continuous Profiling:** Pyroscope offers continuous, always-on profiling, allowing real-time performance analysis.
- **Advanced support:** Support for advanced features such as trace-span profiling, tags/labels, and controlling profiling with code.
- **Higher efficiency:** Less ressource consumed by sending delta instead of cumulative profiles.
- **Enhanced Insights:** With Pyroscope, you gain deeper insights into your application's performance, helping to identify and resolve issues more effectively.
- **Easy Integration:** Migrating to Pyroscope requires minimal changes and provides a more robust profiling solution with little overhead.
- **Customizable Profiling:** Pyroscope enables more granular control over what gets profiled, offering a range of profiling types.

## Migration guide

To view the exact changes made during the migration, refer to our [pull request](https://github.com/grafana/pyroscope/pull/2830). This PR clearly illustrates the differences and the necessary steps to transition from the standard `pprof` library to Pyroscope.

## Conclusion

Migrating to the Pyroscope SDK in a Go application is a straightforward process that significantly enhances profiling capabilities. By following the steps outlined in this guide and reviewing the provided PR, developers can easily transition from the standard `pprof` library to Pyroscope, benefiting from real-time, continuous profiling and advanced performance insights.
