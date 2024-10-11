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
Refer to [Available profiling types]({{< relref "../../view-and-analyze-profile-data/profiling-types#available-profiling-types" >}}) for a list of profile types supported by Go.
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

Then add the following code to your application:

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

`rate` parameter controls the fraction of mutex contention events that are reported in the mutex profile. On average 1/rate events are reported.

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

`rate` parameter controls the fraction of goroutine blocking events that are reported in the blocking profile. The profiler aims to sample an average of one blocking event per rate nanoseconds spent blocked.

## Send data to Pyroscope OSS or Grafana Cloud Profiles

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

To configure the Golang SDK to send data to Pyroscope, replace the `<URL>` placeholder with the appropriate server URL. This could be the Grafana Cloud URL or your own custom Pyroscope server URL.

If you need to send data to Grafana Cloud, you'll have to configure HTTP Basic authentication. Replace `<User>` with your Grafana Cloud stack user and `<Password>` with your Grafana Cloud API key.

If your Pyroscope server has multi-tenancy enabled, you'll need to configure a tenant ID. Replace `<TenantID>` with your Pyroscope tenant ID.

## Golang profiling examples

Check out the following resources to learn more about Golang profiling:

* [Golang examples](https://github.com/grafana/pyroscope/tree/main/examples/language-sdk-instrumentation/golang-push)
* [Golang Demo](https://play.grafana.org/a/grafana-pyroscope-app/single?query=process_cpu%3Acpu%3Ananoseconds%3Acpu%3Ananoseconds%7Bservice_name%3D%22pyroscope-rideshare-go%22%7D&from=now-1h&until=now) showing golang example with tags
