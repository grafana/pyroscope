---
title: "Go (push mode)"
menuTitle: "Go (push mode)"
description: "Instrumenting Golang applications for continuous profiling"
weight: 30
---

# Go (push mode)

Pyroscope uses the standard `runtime/pprof` package to collect profiling data. Refer to
[the official documentation](https://golang.org/doc/diagnostics#profiling) for details.

## How to add Golang profiling to your application

To start profiling a Go application, you need to include our go module in your app:
```
# make sure you also upgrade pyroscope server to version 0.3.1 or higher
go get github.com/pyroscope-io/client/pyroscope
```

Note: If you'd prefer to use Pull mode you can do so using the Grafana Agent.

Then add the following code to your application:

```go
package main

import "github.com/pyroscope-io/client/pyroscope"

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

    // optionally, if authentication is enabled, specify the API key:
    // AuthToken:    os.Getenv("PYROSCOPE_AUTH_TOKEN"),
    
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

## How to add profiling labels to Golang applications

It is possible to add tags (labels) to the profiling data. These tags can be used to filter the data in the UI. We have a custom API that's in line with our other integrations (e.g [Python](/docs/python) or [Ruby](/docs/ruby)) as well as go-native pprof api:

```go
// these two ways of adding tags are equivalent:
pyroscope.TagWrapper(context.Background(), pyroscope.Labels("controller", "slow_controller"), func(c context.Context) {
  slowCode()
})

pprof.Do(context.Background(), pprof.Labels("controller", "slow_controller"), func(c context.Context) {
  slowCode()
})
```

## Mutex Profiling

Mutex profiling is useful for finding sources of contention within your application. It helps you to find out which mutexes are being held by which goroutines.

To enable mutex profiling, you need to add the following code to your application:
```go
runtime.SetMutexProfileFraction(rate)
```

`rate` parameter controls the fraction of mutex contention events that are reported in the mutex profile. On average 1/rate events are reported.

## Block Profiling

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

## Sending data to Phlare with Pyroscope Golang integration

Starting with [weekly-f8](https://hub.docker.com/r/grafana/phlare/tags) you can ingest pyroscope profiles directly to phlare.

```go
pyroscope.Start(pyroscope.Config{
  ApplicationName:   "phlare.golang.app",
  ServerAddress:     "<URL>",
  // Optional HTTP Basic authentication
  BasicAuthUser:     "<User>",
  BasicAuthPassword: "<Password>",
  // Optional Phlare tenant ID
  ScopeOrgID:        "<TenantID>",
  ProfileTypes: []pyroscope.ProfileType{
    pyroscope.ProfileCPU,
    pyroscope.ProfileInuseObjects,
    pyroscope.ProfileAllocObjects,
    pyroscope.ProfileInuseSpace,
    pyroscope.ProfileAllocSpace,
  },
})
```

To configure the Golang integration to send data to Phlare, replace the `<URL>` placeholder with the appropriate server URL. This could be the grafana.com Phlare URL or your own custom Phlare server URL.

If you need to send data to grafana.com, you'll have to configure HTTP Basic authentication. Replace `<User>` with your grafana.com stack user and `<Password>` with your grafana.com API key.

If your Phlare server has multi-tenancy enabled, you'll need to configure a tenant ID. Replace `<TenantID>` with your Phlare tenant ID.


## Golang profiling examples

Check out the following resources to learn more about Golang profiling:
- [Golang examples](https://github.com/pyroscope-io/pyroscope/tree/main/examples/golang-push)
- [Golang Demo](https://demo.pyroscope.io/?query=rideshare-app-golang.cpu%7B%7D) showing golang example with tags
- [Golang blog post](https://pyroscope.io/blog/profiling-go-apps-with-pyroscope)
