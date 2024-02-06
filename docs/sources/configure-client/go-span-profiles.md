---
title: Span profiles with Traces to profiles for Go
menuTitle: Span profiles with Traces to profiles (Go)
description: Learn about and configure Span profiles with Traces to profiles in Grafana for the Go language.
weight: 100
---

# Span profiles with Traces to profiles for Go

![span-profiles screenshot](https://grafana.com/static/img/docs/tempo/profiles/tempo-profiles-Span-link-profile-data-source.png)

## Before you begin

Your applications must be instrumented for profiling and tracing before you can use span profiles. 

* Profiling: Your application must be instrumented with Pyroscopes Go SDK. If you haven't done this yet, please refer to the [Go (push mode)]({{< relref "../configure-client/language-sdks/go_push" >}}) guide.
* Tracing: Your application must be instrumented with OpenTelemetry traces. If you haven't done this yet, please refer to the [OpenTelemetry](https://opentelemetry.io/docs/go/getting-started/) guide.

## OpenTelemetry support

Pyroscope can integrate with distributed tracing systems supporting [**OpenTelemetry**](https://opentelemetry.io/docs/instrumentation/go/getting-started/) standard which allows you to
link traces with the profiling data, and find resource usage for specific lines of code for your trace spans

:::note
 * Only CPU profiling is supported at the moment.
 * Because of how sampling profilers work, spans shorter than the sample interval may not be captured. Go CPU profiler probes stack traces 100 times per second, meaning that spans shorter than 10ms may not be captured.
:::


## Configure the otel-profiling-go package

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
ctx, span := otel.Tracer("tracerName").Start(ctx, "ExampleSpan")
defer span.End()

// Your code goes here.
```

## View the span profiles in Grafana Tempo

To view the span profiles in Grafana Tempo, you need to have a Grafana instance running and a datasource configured to link Trace spans and profiles.

Refer to the [datasource configuration documentation](https://grafana.com/docs/grafana/latest/datasources/tempo/configure-tempo-data-source/#trace-to-profiles) to see how to configure the visualization to link trace spans with profiles.

To use a simple configuration, follow these steps:

1. Select a Pyroscope data source from the Data source drop-down.

2. Optional: Choose any tags to use in the query. If left blank, the default values of service.name and service.namespace are used.

The tags you configure must be present in the spans attributes or resources for a trace to profiles span link to appear. You can optionally configure a new name for the tag. This is useful for example if the tag has dots in the name and the target data source doesn’t allow using dots in labels. In that case you can for example remap service.name to service_name.

3. Select one or more profile types to use in the query. Select the drop-down and choose options from the menu.

The profile type or app must be selected for the query to be valid. Grafana doesn’t show any data if the profile type or app isn’t selected when a query runs.

![span-profiles configuration](https://grafana.com/static/img/docs/tempo/profiles/Tempo-data-source-profiles-Settings.png)

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