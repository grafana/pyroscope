---
title: "Go (push mode)"
menuTitle: "Go (push mode)"
description: "Instrumenting Golang applications for continuous profiling"
weight: 10
aliases:
  - /docs/phlare/latest/configure-client/language-sdks/go_push
---

# Go (push mode)

Our Go Profiler is a cutting-edge tool designed to optimize Golang applications. By integrating with Pyroscope, it offers developers an in-depth view of their Go codebase, enabling real-time performance analysis. This powerful tool is crucial for pinpointing inefficiencies, streamlining code execution, and ensuring peak performance in Go applications.

Pyroscope uses the standard `runtime/pprof` package to collect profiling data. Refer to [the official documentation](https://golang.org/doc/diagnostics#profiling) for details.


## Before you Begin

### Set Up a Pyroscope Server

To capture and analyze profiling data, set up a Pyroscope server. This can be:

* A **local server** for development, or
* A **remote server** for production use.

For installation instructions, see our [Get Started]({{< relref "../../get-started" >}}) guide.


### Using Grafana Cloud Profiles

Grafana Cloud Profiles is a hosted Pyroscope service. It provides a fully managed Pyroscope server, so you don't have to worry about installing and maintaining your own server. It also provides a hosted Grafana instance for visualizing your profiling data. For more information, see [Grafana Cloud Profiles](/products/cloud/profiles-for-continuous-profiling/).

<!-- TODO: add a section like "Learn more about reading flamegraphs and using our product" once it's ready -->


## Configure the Go client

To start profiling a Go application, you need to include our go module in your app:

```
go get github.com/grafana/pyroscope-go
```

Note: If you'd prefer to use Pull mode you can do so using the [Grafana Agent](../grafana-agent/).

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

It is possible to add tags (labels) to the profiling data. These tags can be used to filter the data in the UI. We have a custom API that's in line with the go-native pprof api:

```go
// these two ways of adding tags are equivalent:
pyroscope.TagWrapper(context.Background(), pyroscope.Labels("controller", "slow_controller"), func(c context.Context) {
  slowCode()
})

pprof.Do(context.Background(), pprof.Labels("controller", "slow_controller"), func(c context.Context) {
  slowCode()
})
```

### Mutex Profiling

Mutex profiling is useful for finding sources of contention within your application. It helps you to find out which mutexes are being held by which goroutines.

To enable mutex profiling, you need to add the following code to your application:

```go
runtime.SetMutexProfileFraction(rate)
```

`rate` parameter controls the fraction of mutex contention events that are reported in the mutex profile. On average 1/rate events are reported.

### Block Profiling

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

* [Golang examples](https://github.com/grafana/pyroscope-go/blob/main/example/main.go)
* [Golang Demo](https://demo.pyroscope.io/?query=rideshare-app-golang.cpu%7B%7D) showing golang example with tags
* [Golang blog post](https://pyroscope.io/blog/profiling-go-apps-with-pyroscope)
