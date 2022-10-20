---
title: "Go"
menuTitle: "Go"
description: ""
weight: 10
---

# Go

Go natively support [pprof endpoints](https://pkg.go.dev/net/http/pprof). To activate it, you need to add the following lines to your code:

```go
import _ "net/http/pprof"
```

By default pprof endpoint will be exposed on the [`http.DefaultServeMux`](https://pkg.go.dev/net/http#pkg-variables).
If you're not using the default mux, you can register pprof handlers on it instead:

```go
import pprof "net/http/pprof"

mux.Handle("/debug/pprof/", pprof.Index)
```

> **Note:** You should not expose pprof on public endpoints.

Finally, if you are not running a `http.Server` on your application you will need to start one in your main function:

```go
go func() {
    log.Println(http.ListenAndServe("localhost:6060", nil))
}()
```

## Block Profiling

The `block` profile in Go lets you analyze how much time your program spends waiting on the blocking operations (select, channel op, mutex, cond).
The block profiler is disabled by default. You can enable it by passing a `rate > 0` as shown below.

```go
import "runtime"

func init(){
    runtime.SetBlockProfileRate(rate)
}
```

A rate of 1 will capture a stack trace every other block operation, but your runtime performance will take a hit.
A rate superior to or equal to 2 will set the sampling rate in nanoseconds to sample all blocking events lasting that duration or longer.

## Mutex Profiling

Go 1.8 introduces a new profile, the contended mutex profile, that allows you to capture a fraction of the stack traces of goroutines with contended mutexes.
When you think your CPU is not fully utilized due to a mutex contention, use this profile.
Mutex profile is not enabled by default, see [`runtime.SetMutexProfileFraction`](https://pkg.go.dev/runtime@master#SetMutexProfileFraction) to enable.

```go
import "runtime"

func init(){
    runtime.SetMutexProfileFraction(rate)
}
```

Rate controls the fraction of mutex contention events that are reported in the mutex profile. On average 1/rate events are reported.
To turn off profiling entirely, pass rate 0 (default).

## Combining I/O and CPU Mode Profiles

To enable both CPU mode and mixed I/O and CPU profiling enabled using [`fgprof`](https://github.com/felixge/fgprof), use the following example snippet:

```go
import (
    "fmt"
    "listen"
    _ "net/http/pprof"

    fgprof "github.com/felixge/fgprof"
)

func main() {
    http.DefaultServeMux.Handle("/debug/fgprof", fgprof.Handler())
    go func() {
        log.Println(http.ListenAndServe(":6060", nil))
    }()
}
```

You are not required to start a separated `http.Server` if your application is already doing so.

You'll also need to activate scraping for this new endpoint using the following snippet:

```yaml
scrape_configs:
  - job_name: 'default'
    profiling_config:
      pprof_config:
        fgprof:
          path: /debug/fgprof
          delta: true
          enabled: true
```
