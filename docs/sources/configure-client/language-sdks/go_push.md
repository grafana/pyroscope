---
title: "Go (push mode)"
menuTitle: "Go (push mode)"
description: "Instrumenting Golang applications for continuous profiling."
weight: 10
aliases:
  - /docs/phlare/latest/configure-client/language-sdks/go_push
---

# Go (push mode)

Our Go Profiler is a cutting-edge tool designed to optimize Golang applications.
By integrating with Pyroscope, the profiler offers developers an in-depth view of their Go codebase, enabling real-time performance analysis.
This powerful tool is crucial for pinpointing inefficiencies, streamlining code execution, and ensuring peak performance in Go applications.

Pyroscope uses the standard `runtime/pprof` package to collect profiling data.
Refer to [the official documentation](https://golang.org/doc/diagnostics#profiling) for details.

{{< admonition type="note" >}}
Refer to [Available profiling types](https://grafana.com/docs/pyroscope/latest/configure-client/profile-types/) for a list of profile types supported by Go.
{{< /admonition >}}

## Before you begin

To capture and analyze profiling data, you need either a hosted Pyroscope OSS server or a hosted [Pyroscope instance with Grafana Cloud Profiles](/products/cloud/profiles-for-continuous-profiling/) (requires a free Grafana Cloud account).

The Pyroscope server can be a local server for development or a remote server for production use.


## Configure the Go client

To start profiling a Go application, you need to include the Go module in your app:

```go
go get github.com/grafana/pyroscope-go
```

{{% admonition type="note" %}}
If you'd prefer to use Pull mode you can do so using [Grafana Alloy](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/grafana-alloy/).
{{% /admonition %}}

Add the following code to your application:

```go
package main

import "github.com/grafana/pyroscope-go"

func main() {
  // These 2 lines are only required if you're using mutex or block profiling
  // Read the explanation below for how to set these rates:
  runtime.SetMutexProfileFraction(5)
  runtime.SetBlockProfileRate(5)

  pyroscope.Start(pyroscope.Config{
    ApplicationName: "simple.golang.app",

    // replace this with the address of pyroscope server
    ServerAddress:   "http://pyroscope-server:4040",

    // you can disable logging by setting this to nil
    Logger:          pyroscope.StandardLogger,

    // you can provide static tags via a map:
    Tags:            map[string]string{"hostname": os.Getenv("HOSTNAME")},

    ProfileTypes: []pyroscope.ProfileType{
      // these profile types are enabled by default:
      pyroscope.ProfileCPU,
      pyroscope.ProfileAllocObjects,
      pyroscope.ProfileAllocSpace,
      pyroscope.ProfileInuseObjects,
      pyroscope.ProfileInuseSpace,

      // these profile types are optional:
      pyroscope.ProfileGoroutines,
      pyroscope.ProfileMutexCount,
      pyroscope.ProfileMutexDuration,
      pyroscope.ProfileBlockCount,
      pyroscope.ProfileBlockDuration,
    },
  })

  // your code goes here
}
```

Alternatively, if you want more control over the profiling process, you can manually handle the profiler initialization and termination:

```go
  profiler, err := pyroscope.Start(pyroscope.Config{
    // omitted for brevity 
  })
  if err != nil {
    // the only reason this would fail is if the configuration is not valid
    log.Fatalf("failed to start Pyroscope: %v", err)
  }
  defer profiler.Stop()

  // your code goes here
}
```

This approach may be necessary if you need to ensure that the last profile is sent before the application exits.

### Add profiling labels to your application

You can add tags (labels) to the profiling data. These tags can be used to filter the data in the UI. There is a custom API that's in line with the go-native pprof API:

```go
// these two ways of adding tags are equivalent:
pyroscope.TagWrapper(context.Background(), pyroscope.Labels("controller", "slow_controller"), func(c context.Context) {
  slowCode()
})

pprof.Do(context.Background(), pprof.Labels("controller", "slow_controller"), func(c context.Context) {
  slowCode()
})
```

### Mutex profiling

Mutex profiling is useful for finding sources of contention within your application. It helps you find out which mutexes are being held by which goroutines.

To enable mutex profiling, you need to add the following code to your application:

```go
runtime.SetMutexProfileFraction(rate)
```

The `rate` parameter controls the fraction of mutex contention events that are reported in the mutex profile. On average, 1/rate events are reported.

### Block profiling

Block profiling lets you analyze how much time your program spends waiting on the blocking operations such as:

* select
* channel send/receive
* semacquire
* notifyListWait

To enable block profiling, you need to add the following code to your application:

```go
runtime.SetBlockProfileRate(rate)
```

The `rate` parameter controls the fraction of goroutine blocking events that are reported in the blocking profile.
The profiler aims to sample an average of one blocking event per rate nanoseconds spent blocked.

## Send data to Pyroscope OSS or Grafana Cloud Profiles

To configure the Golang SDK to send data to Pyroscope, replace the `<URL>` placeholder with the appropriate server URL.
This could be the Grafana Cloud URL or your own custom Pyroscope server URL.

If you need to send data to Grafana Cloud, you'll have to configure HTTP Basic authentication.
Replace `<User>` with your Grafana Cloud stack user and `<Password>` with your Grafana Cloud API key.

If your Pyroscope server has multi-tenancy enabled, you'll need to configure a tenant ID.
Replace `<TenantID>` with your Pyroscope tenant ID.

```go
pyroscope.Start(pyroscope.Config{
  ApplicationName:   "example.golang.app",
  ServerAddress:     "<URL>",
  // Optional HTTP Basic authentication
  BasicAuthUser:     "<User>",
  BasicAuthPassword: "<Password>",
  // Optional Pyroscope tenant ID (only needed if using multi-tenancy). Not needed for Grafana Cloud.
  // TenantID:          "<TenantID>",
  ProfileTypes: []pyroscope.ProfileType{
    pyroscope.ProfileCPU,
    pyroscope.ProfileInuseObjects,
    pyroscope.ProfileAllocObjects,
    pyroscope.ProfileInuseSpace,
    pyroscope.ProfileAllocSpace,
  },
})
```

### Option: Use `DisableGCRuns` for handling increased memory usage

Pyroscope may require additional resources when tracking a lot of objects. For example, a Go service that indexes large amounts of data requires more memory.
This tracking can lead to higher CPU usage and potential CPU throttling.

You can use `DisableGCRuns` in your Go configuration to disable automatic runtimes.
If this flag is activated, there is less GC running and therefore less CPU resources spent.
However, the heap profile may be less precise.

#### Background

In Go's pprof heap profiling, forcing garbage collection (GC) ensures accurate memory usage snapshots by removing uncollected objects.
Without this step, the heap profile may include memory that has been allocated but is no longer in use--objects that stay in memory simply because they haven't been collected yet.
This can mask or mimic memory leaks and introduce bias into the profiles, complicating their analysis.
Therefore, Pyroscope defaults to forcing GC every time a heap profile is collected.

However, in some cases, forcing GC can increase CPU usage, especially if there are many live objects in the heap.
This issue is reflected by the appearance of the `runtime.GC` function in the CPU profile.
If the problem has manifested, and some inaccuracy in the heap profile is acceptable, then it is advisable to disable this option to avoid performance degradation.

#### Activate `DisableGCRuns`

Add `DisableGCRuns: true` to the `pyroscope.Start(pyroscope.Config)` block.

```go
pyroscope.Start(pyroscope.Config{
  ApplicationName:   "example.golang.app",
  ServerAddress:     "<URL>",
  // Disable automatic runtime.GC runs between getting the heap profiles.
		DisableGCRuns:   true,
```

## Golang profiling examples

Check out the following resources to learn more about Golang profiling:

* [Golang examples](https://github.com/grafana/pyroscope/tree/main/examples/language-sdk-instrumentation/golang-push)
* [Golang Demo](https://play.grafana.org/a/grafana-pyroscope-app/single?query=process_cpu%3Acpu%3Ananoseconds%3Acpu%3Ananoseconds%7Bservice_name%3D%22pyroscope-rideshare-go%22%7D&from=now-1h&until=now) showing golang example with tags
