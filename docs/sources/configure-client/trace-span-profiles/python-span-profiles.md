---
title: Span profiles with Traces to profiles for Python
menuTitle: Span profiles with Traces to profiles (Python)
description: Learn about and configure Span profiles with Traces to profiles in Grafana for Python applications.
weight: 104
---

# Span profiles with Traces to profiles for Python

Span Profiles represent a major shift in profiling methodology, enabling deeper analysis of both tracing and profiling data.
Traditional continuous profiling provides an application-wide view over fixed intervals.
In contrast, Span Profiles delivers focused, dynamic analysis on specific execution scopes within applications, such as individual requests or specific trace spans.

This shift enables a more granular view of performance, enhancing the utility of profiles by linking them directly with traces for a comprehensive understanding of application behavior. As a result, engineering teams can more efficiently identify and address performance bottlenecks.

To learn more about Span Profiles, refer to [Combining tracing and profiling for enhanced observability: Introducing Span Profiles](/blog/2024/02/06/combining-tracing-and-profiling-for-enhanced-observability-introducing-span-profiles/).

![span-profiles screenshot](https://grafana.com/static/img/docs/tempo/profiles/tempo-profiles-Span-link-profile-data-source.png)

Pyroscope integrates with distributed tracing systems supporting the [**OpenTelemetry**](https://opentelemetry.io/docs/languages/python/getting-started/) standard.
This integration lets you link traces with the profiling data and find resource usage for specific lines of code for your trace spans.

{{< admonition type="note" >}}
* Only CPU profiling is supported at the moment.
* Because of how sampling profilers work, spans shorter than the sample interval may not be captured.
{{< /admonition >}}

To use Span Profiles, you need to:

* [Configure Pyroscope to send profiling data]({{< relref "../../configure-client" >}})
* Configure a client-side package to link traces and profiles: [Python](https://github.com/grafana/otel-profiling-python)
* [Configure the Tempo data source in Grafana or Grafana Cloud to discover linked traces and profiles](/docs/grafana-cloud/connect-externally-hosted/data-sources/tempo/configure-tempo-data-source/)

## Before you begin

Your applications must be instrumented for profiling and tracing before you can use span profiles.

* Profiling: Your application must be instrumented with Pyroscope's Python instrumentation library. Refer to the [Python]({{< relref "../language-sdks/python" >}}) guide for instructions.
* Tracing: Your application must be instrumented with OpenTelemetry traces. Refer to the [OpenTelemetry](https://opentelemetry.io/docs/languages/python/getting-started/) guide for isntructions.

## Configure the `pyroscope-otel` package

To start collecting Span Profiles for your Python application, you need to include [pyroscope-otel](https://github.com/grafana/otel-profiling-python) in your code.

This package provides a [`SpanProcessor`](https://github.com/open-telemetry/opentelemetry-python/blob/d213e02941039d4383abc3608b75404ce84725b1/opentelemetry-sdk/src/opentelemetry/sdk/trace/__init__.py#L85) implementation, which connects the two telemetry signals (traces and profiles) together.

```shell
pip install pyroscope-otel
```

Next, create and register the `PyroscopeSpanProcessor`:
```python
# import span processor
from pyroscope-otel import PyroscopeSpanProcessor

# obtain a OpenTelemetry tracer provider
from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
provider = TracerProvider()

# register the span processor
provider.add_span_processor(PyroscopeSpanProcessor())

# register the tracer provider
trace.set_tracer_provider(provider)
```

With the span processor registered, spans created automatically (for example, HTTP handlers) and manually will have profiling data associated with them.

## View the span profiles in Grafana Tempo

To view the span profiles in Grafana Tempo, you need to have a Grafana instance running and a data source configured to link traces and profiles.

Refer to the [data source configuration documentation](https://grafana.com/docs/grafana/<GRAFANA_VERSION>/datasources/tempo/configure-tempo-data-source) to see how to configure the visualization to link traces with profiles.

## Examples

Check out these demo applications for span profiles:
- [Python example](https://github.com/grafana/pyroscope/tree/main/examples/tracing/python)
- [Other examples](https://github.com/grafana/pyroscope/tree/main/examples/tracing/tempo) in multiple languages
