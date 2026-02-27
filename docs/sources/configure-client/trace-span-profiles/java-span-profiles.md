---
title: Span profiles with Traces to profiles for Java
menuTitle: Span profiles with Traces to profiles (Java)
description: Learn about and configure Span profiles with Traces to profiles in Grafana for the Java language.
weight: 101
---

# Span profiles with Traces to profiles for Java

Span Profiles represent a major shift in profiling methodology, enabling deeper analysis of both tracing and profiling data.
Traditional continuous profiling provides an application-wide view over fixed intervals.
In contrast, Span Profiles delivers focused, dynamic analysis on specific execution scopes within applications, such as individual requests or specific trace spans.

This shift enables a more granular view of performance, enhancing the utility of profiles by linking them directly with traces for a comprehensive understanding of application behavior. As a result, engineering teams can more efficiently identify and address performance bottlenecks.

To learn more about Span Profiles, refer to [Combining tracing and profiling for enhanced observability: Introducing Span Profiles](/blog/2024/02/06/combining-tracing-and-profiling-for-enhanced-observability-introducing-span-profiles/).

![span-profiles screenshot](https://grafana.com/static/img/docs/tempo/profiles/tempo-profiles-Span-link-profile-data-source.png)

Pyroscope integrates with distributed tracing systems supporting the [**OpenTelemetry**](https://opentelemetry.io/docs/instrumentation/java/getting-started/) standard.
This integration lets you link traces with the profiling data and find resource usage for specific lines of code for your trace spans.

{{< admonition type="note" >}}
Java span profiles support CPU (`itimer`, `cpu`) and wall profile types. For other languages, only CPU profiling is supported.
Because of how sampling profilers work, spans shorter than the sample interval may not be captured.
{{< /admonition >}}

To use Span Profiles, you need to:

* [Configure Pyroscope to send profiling data](../../)
* Configure a client-side package to link traces and profiles: [Java](https://github.com/grafana/otel-profiling-java)
* [Configure the Tempo data source in Grafana or Grafana Cloud to discover linked traces and profiles](/docs/grafana-cloud/connect-externally-hosted/data-sources/tempo/configure-tempo-data-source/)

## Before you begin

Your applications must be instrumented for profiling and tracing before you can use span profiles.

* Profiling: Your application must be instrumented with Pyroscope's Java client SDK. Refer to the [Java](../../language-sdks/java/) guide for instructions.
* Tracing: Your application must be instrumented with OpenTelemetry traces. Refer to the [OpenTelemetry](https://opentelemetry.io/docs/languages/java/getting-started/) guide for instructions.

## Supported profile types

Java span profiles support two profile types, controlled by the `PYROSCOPE_PROFILER_EVENT` environment variable (or the `pyroscope.profiler.event` property):

- **CPU profiles** (`itimer` or `cpu` event): Capture time the application spends actively executing on the CPU. Use CPU profiles to identify compute-bound bottlenecks.
- **Wall profiles** (`wall` event): Capture elapsed wall-clock time, including time spent sleeping, waiting on I/O, locks, or network calls. Use wall profiles to identify latency-bound bottlenecks where spans spend significant time off-CPU.

The same `PYROSCOPE_PROFILER_EVENT` setting applies whether you use span profiles or continuous profiling without trace correlation.

## Configure the `otel-profiling-java` package

To start collecting Span Profiles for your Java application, you need to include [otel-profiling-java](https://github.com/grafana/otel-profiling-java) as an extension
for the [OpenTelemetry Java agent](https://opentelemetry.io/docs/zero-code/java/agent/).

Assuming you have this sample application Docker image:

```Dockerfile
# [...]

EXPOSE 5000

CMD ["java", "-Dserver.port=5000", "-jar", "./my-app.jar" ]
```

By adding the OTel Java agent and the Pyroscope OTel Java Agent extension, you can enrich your profiles with span IDs. This makes it possible to query for span-specific profiling data stored in Tempo:

```Dockerfile
# [...]

EXPOSE 5000

## Add required libraries
ADD https://github.com/open-telemetry/opentelemetry-java-instrumentation/releases/download/v1.17.0/opentelemetry-javaagent.jar opentelemetry-javaagent.jar
ADD https://github.com/grafana/otel-profiling-java/releases/download/v1.0.4/pyroscope-otel.jar pyroscope-otel.jar

ENV PYROSCOPE_APPLICATION_NAME=my-app
ENV PYROSCOPE_FORMAT=jfr
ENV PYROSCOPE_PROFILING_INTERVAL=10ms
ENV PYROSCOPE_PROFILER_EVENT=itimer
ENV PYROSCOPE_PROFILER_LOCK=10ms
ENV PYROSCOPE_PROFILER_ALLOC=512k
ENV PYROSCOPE_UPLOAD_INTERVAL=15s
ENV OTEL_JAVAAGENT_EXTENSIONS=./pyroscope-otel.jar
ENV OTEL_PYROSCOPE_ADD_PROFILE_URL=false
ENV OTEL_PYROSCOPE_ADD_PROFILE_BASELINE_URL=false
ENV OTEL_PYROSCOPE_START_PROFILING=true

## Useful for debugging
# ENV PYROSCOPE_LOG_LEVEL=debug

## Those environment variables need to be overwritten at runtime, if you are using Grafana Cloud
ENV PYROSCOPE_SERVER_ADDRESS=http://localhost:4040
# ENV PYROSCOPE_BASIC_AUTH_USER=123     ## Grafana Cloud Username
# ENV PYROSCOPE_BASIC_AUTH_PASSWORD=glc_secret ## Grafana Cloud Password / API Token

## Add the opentelemetry java agent
CMD ["java", "-Dserver.port=5000", "-javaagent:./opentelemetry-javaagent.jar", "-jar", "./my-app.jar" ]
```

To use wall profiles instead of CPU profiles, set `PYROSCOPE_PROFILER_EVENT` to `wall`:

```Dockerfile
ENV PYROSCOPE_PROFILER_EVENT=wall
```

### Available configuration options

| Flag                             | Description                                                                                                                                                                                                                                                                                                             | Default |
|----------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|---------|
| `otel.pyroscope.start.profiling` | Boolean flag to start PyroscopeAgent. Set to false if you want to start the PyroscopeAgent manually.                                                                                                                                                                                                                    | `true`  |
| `otel.pyroscope.root.span.only`  | Boolean flag. When enabled, the tracer will annotate only the first span created locally (the root span), but the profile will include samples of all the nested spans. This may be helpful in case if the trace consists of multiple spans shorter than 10ms and profiler can't collect and annotate samples properly. | `true`  |
| `otel.pyroscope.add.span.name`   | Boolean flag. Controls whether the span name added to profile labels.                                                                                                                                                                                                                                                   | `true`  |

## View the span profiles in Grafana

To view the span profiles in Grafana, you need to have a Grafana instance running and a Tempo data source configured to link trace spans and profiles.

Refer to the [data source configuration documentation](https://grafana.com/docs/grafana/<GRAFANA_VERSION>/datasources/tempo/configure-tempo-data-source/) to see how to configure the visualization to link trace spans with profiles.

To use a simple configuration, follow these steps:

1. Select a Pyroscope data source from the Data source drop-down.

2. Optional: Choose any tags to use in the query. If left blank, the default values of `service.name` and `service.namespace` are used.

   The tags you configure must be present in the spans attributes or resources for a trace to profiles span link to appear. You can optionally configure a new name for the tag. This is useful for example if the tag has dots in the name and the target data source doesn't allow using dots in labels. In that case you can for example remap `service.name` to `service_name`.

3. Select one or more profile types to use in the query. Select the drop-down and choose options from the menu.

   If your application uses wall profiles (`PYROSCOPE_PROFILER_EVENT=wall`), select the `wall` profile type. If your application uses CPU profiles (`itimer` or `cpu`), select the `cpu` profile type.

   The profile type or app must be selected for the query to be valid. Grafana doesn't show any data if the profile type or app isn’t selected when a query runs.

   ![span-profiles configuration](https://grafana.com/static/img/docs/tempo/profiles/Tempo-data-source-profiles-Settings.png)

## Examples

Check out the examples directory for complete demo applications that show tracing integration features:

- [Java example with CPU profiles](https://github.com/grafana/pyroscope/tree/main/examples/tracing/java)
- [Java example with wall profiles](https://github.com/grafana/pyroscope/tree/main/examples/tracing/java-wall)
