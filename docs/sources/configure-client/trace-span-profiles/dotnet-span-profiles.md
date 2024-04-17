---
title: Span profiles with Traces to profiles for .NET
menuTitle: Span profiles with Traces to profiles (.NET)
description: Learn about and configure Span profiles with Traces to profiles in Grafana for .NET applications.
weight: 103
---

# Span profiles with Traces to profiles for .NET

![span-profiles screenshot](https://grafana.com/static/img/docs/tempo/profiles/tempo-profiles-Span-link-profile-data-source.png)

## Before you begin

Your applications must be instrumented for profiling and tracing before you can use span profiles.

* Profiling: Your application must be instrumented with Pyroscope's .NET instrumentation library. If you haven't done this yet, please refer to the [.NET]({{< relref "../language-sdks/dotnet" >}}) guide.
* Tracing: Your application must be instrumented with OpenTelemetry traces. If you haven't done this yet, please refer to the [OpenTelemetry](https://opentelemetry.io/docs/net/getting-started/) guide.

{{< admonition type="note" >}}
Currently only [OpenTelemetry manual instrumentation](https://opentelemetry.io/docs/languages/net/instrumentation/) is supported for span profiles in .NET.
This limitation is there because Pyroscope's .NET profiler and OpenTelemetry's auto instrumentation are based on separate .NET CLR profilers.
{{< /admonition >}}

## OpenTelemetry support

Pyroscope can integrate with distributed tracing systems supporting the [**OpenTelemetry**](https://opentelemetry.io/docs/instrumentation/net/getting-started/) standard which allows you to
link traces with the profiling data, and find resource usage for specific lines of code for your trace spans

{{< admonition type="note" >}}
* Only CPU profiling is supported at the moment.
* Because of how sampling profilers work, spans shorter than the sample interval may not be captured.
{{< /admonition >}}


## Configure the `Pyroscope.OpenTelemetry` package

To start collecting Span Profiles for your .NET application, you need to include [Pyroscope.OpenTelemetry](https://github.com/grafana/pyroscope-dotnet/tree/main/Pyroscope/Pyroscope.OpenTelemetry) in your code.

This package provides a [`SpanProcessor`](https://github.com/open-telemetry/opentelemetry-dotnet/blob/main/src/OpenTelemetry/BaseProcessor.cs) implementation, which connects the two telemetry signals (traces and profiles) together.

```shell
dotnet add package Pyroscope.OpenTelemetry
```

Next, we need to create and register the `PyroscopeSpanProcessor`:
```csharp
builder.Services.AddOpenTelemetry()
    .WithTracing(b =>
    {
        b
        .AddAspNetCoreInstrumentation()
        .AddConsoleExporter()
        .AddOtlpExporter()
        .AddProcessor(new Pyroscope.OpenTelemetry.PyroscopeSpanProcessor());
    });
```

With the span processor registered, spans created automatically (e.g., for our HTTP handlers) and manually (`ActivitySource.StartActivity()`) will have profiling data associated with them.

```

## View the span profiles in Grafana Tempo

To view the span profiles in Grafana Tempo, you need to have a Grafana instance running and a data source configured to link traces and profiles.

Refer to the [data source configuration documentation](/docs/grafana/datasources/tempo/configure-tempo-data-source) to see how to configure the visualization to link traces with profiles.

## Examples

Check out the [examples](https://github.com/grafana/pyroscope/tree/main/examples/tracing/tempo) directory for a complete demo application of span profiles in multiple languages.
