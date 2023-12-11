# Migrating from standard pprof to Pyroscope in a Go application

This README provides a comprehensive guide on migrating from the standard pprof library to Pyroscope in a Go application. The example demonstrates the transition within a detective-themed Go application, enhancing the process of profiling with Pyroscope's advanced capabilities.

## Changes made

### Pre-Pyroscope setup

Originally, our `main.go` file used the standard `net/http/pprof` package for profiling. This setup is common and straightforward but lacks continuous profiling and real-time analysis capabilities.

The functions `gatherClues`, `analyzeEvidence`, `interviewWitnesses`, `chaseSuspect`, and `solveMystery` were designed to simulate different CPU loads using busy loops. This approach provided a clear representation of various workloads in the pprof profiler.

### Post-Pyroscope Migration

To leverage the advanced features of Pyroscope, we made the following changes:

1. **Removed Standard pprof Import:** The `_ "net/http/pprof"` import was removed, as Pyroscope replaces its functionality.

2. **Added Pyroscope SDK:** We installed the Pyroscope module using `go get github.com/grafana/pyroscope-go` and imported it in our `main.go`.

3. **Configured Pyroscope:** Inside the `main()` function, we set up Pyroscope using the `pyroscope.Start()` method with the following configuration:
   - Application name and server address.
   - Logger configuration.
   - Tags for additional metadata.
   - Profile types to be captured.

4. **Removed pprof Server:** The goroutine for the pprof HTTP server (`http.ListenAndServe`) was removed, as Pyroscope handles profiling internally.

## Benefits of Using Pyroscope

- **Continuous Profiling:** Pyroscope offers continuous, always-on profiling, allowing real-time performance analysis.
- **Enhanced Insights:** With Pyroscope, you gain deeper insights into your application's performance, helping to identify and resolve issues more effectively.
- **Easy Integration:** Migrating to Pyroscope requires minimal changes and provides a more robust profiling solution with little overhead.
- **Customizable Profiling:** Pyroscope enables more granular control over what gets profiled, offering a range of profiling types.

## Migration Guide

To view the exact changes made during the migration, refer to our pull request: [Placeholder for PR Link]. This PR clearly illustrates the differences and necessary steps to transition from standard pprof to Pyroscope.

## Conclusion

Migrating to Pyroscope SDK in a Go application is a straightforward process that significantly enhances profiling capabilities. By following the steps outlined in this guide and reviewing the provided PR, developers can easily switch from standard pprof to Pyroscope, benefiting from real-time, continuous profiling and advanced performance insights.


