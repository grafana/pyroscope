---
title: Go span profiles with trace-to-profiles
sidebar_label: Go span profiles
description: Go span profiles with trace-to-profiles
weight: 100
---

# Go span profiles with trace-to-profiles


## Prerequisites

1. Profiling: You will need to have your application instrumented with Pyroscopes Go SDK. If you haven't done this yet, please refer to the [Go (push mode)](../configure-client/language-sdks/go_push.md) guide.
2. Tracing: You will need to have your application instrumented with OpenTelemetry traces. If you haven't done this yet, please refer to the [OpenTelemetry](https://opentelemetry.io/docs/go/getting-started/) guide.

## OpenTelemetry support

Pyroscope can integrate with distributed tracing systems supporting [**OpenTelemetry**](https://opentelemetry.io/docs/instrumentation/go/getting-started/) standard which allows you to
link traces with the profiling data, and find resource usage for specific lines of code for your trace spans

:::note
 * Only CPU profiling is supported at the moment.
 * Because of how sampling profilers work, spans shorter than the sample interval may not be captured. Go CPU profiler probes stack traces 100 times per second, meaning that spans shorter than 10ms may not be captured.
:::


## Configuring the tracer

To start collecting Span Profiles for your Go application, you need to include [otel-profiling-go](https://github.com/pyroscope-io/otel-profiling-go) in your code. 

This package is a `TracerProvider` implementation, that labels profiling data with span IDs which makes it possible to query for span-specific profiling data in Grafana Tempo UI. 

```shell
# Make sure you also upgrade pyroscope server to version 0.14.0 or higher.
go get github.com/grafana/otel-profiling-go
```

Next you need to create and configure the tracer provider:
```go
package main

import (
	otelpyroscope "github.com/grafana/otel-profiling-go"
	"github.com/grafana/pyroscope-go"
)

func main() {
	// Initialize your tracer provider as usual.
	tp := initTracer()

	// Wrap it with otelpyroscope tracer provider.
	otel.SetTracerProvider(otelpyroscope.NewTracerProvider(tp))

	// If you're using Pyroscope Go SDK, initialize pyroscope profiler.
	_, _ = pyroscope.Start(pyroscope.Config{
		ApplicationName: "my-service",
		ServerAddress:   "http://localhost:4040",
	})

	// Your code goes here.
}
```

Now that we set up the tracer, we can create a new trace from anywhere and the profiler will automatically capture profiles for it.
```go
ctx, span := otel.Tracer("tracerName").Start(context.Background(), "ExampleSpan")
defer span.End()

// Your code goes here.
```

## Examples

Check out the [examples](https://github.com/grafana/pyroscope/tree/main/examples/tracing/tempo) directory in our repository to
find a complete example application that demonstrates tracing integration features.

<!-- ## Using tracing exemplars manually

If you're not using open telemetry integration you can still use exemplars storage to store profiles associated with some execution context (e.g individual HTTP / GRPC request). To create exemplars you need to tag specific parts of your code with a special `profile_id` tag, for example, in golang you could do this:
```golang
pprof.Do(ctx, pprof.Labels("profile_id", "8474e98b95013e4f"), func(ctx context.Context) {
  slowRequest()
})
```

`"8474e98b95013e4f"` can be any ID that you use to identify execution contexts (individual HTTP / GRPC requests). -->