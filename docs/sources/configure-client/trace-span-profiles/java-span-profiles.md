---
title: Span profiles with Traces to profiles for Java
menuTitle: Span profiles with Traces to profiles (Java)
description: Learn about and configure Span profiles with Traces to profiles in Grafana for the Java language.
weight: 100
---

# Span profiles with Traces to profiles for Java

![span-profiles screenshot](https://grafana.com/static/img/docs/tempo/profiles/tempo-profiles-Span-link-profile-data-source.png)

## Before you begin

Your applications must be instrumented for profiling and tracing before you can use span profiles.

* Profiling: Your application must be instrumented with Pyroscopes Java client SDK. If you haven't done this yet, please refer to the [Java ()]({{< relref "../language-sdks/java" >}}) guide.
* Tracing: Your application must be instrumented with OpenTelemetry traces. If you haven't done this yet, please refer to the [OpenTelemetry](https://opentelemetry.io/docs/java/getting-started/) guide.

## OpenTelemetry support

Pyroscope can integrate with distributed tracing systems supporting [**OpenTelemetry**](https://opentelemetry.io/docs/instrumentation/java/getting-started/) standard which allows you to
link traces with the profiling data, and find resource usage for specific lines of code for your trace spans

{{< admonition type="note" >}}
 * Only CPU profiling is supported at the moment.
 * Because of how sampling profilers work, spans shorter than the sample interval may not be captured. Java CPU profiler probes stack traces 100 times per second, meaning that spans shorter than 10ms may not be captured.
{{< /admonition >}}


## Configure the otel-profiling-java package

To start collecting Span Profiles for your Java application, you need to include [otel-profiling-java](https://github.com/pyroscope-io/otel-profiling-java) in your code.

This package is a `TracerProvider` implementation, that labels profiling data with span IDs which makes it possible to query for span-specific profiling data in Grafana Tempo UI.

```shell
java -jar ./build/libs/rideshare-1.0-SNAPSHOT.jar \
    -javaagent:./opentelemetry-javaagent.jar \
    -Dotel.javaagent.extensions=./pyroscope-otel.jar \
    -Dotel.pyroscope.start.profiling=true \
    -Dpyroscope.application.name=ride-sharing-app-java-instrumentation  \
    -Dpyroscope.format=jfr \
    -Dpyroscope.profiler.event=itimer \
    -Dpyroscope.server.address=$PYROSCOPE_SERVER_ADDRESS \
    # rest of your otel-java-instrumentation configuration

```

Next, you need to create and configure the tracer provider:
```java
implementation("io.pyroscope:otel:0.10.1.1")

// obtain SdkTracerProviderBuilder
SdkTracerProviderBuilder tpBuilder = ...

// Add PyroscopeOtelSpanProcessor to SdkTracerProviderBuilder
PyroscopeOtelConfiguration pyroscopeTelemetryConfig = new PyroscopeOtelConfiguration.Builder()
  .setAddSpanName(true)
  .setRootSpanOnly(true)
  .build();
tpBuilder.addSpanProcessor(new PyroscopeOtelSpanProcessor(pyroscopeOtelConfig));

```

Now that we set up the tracer, we can create a new trace from anywhere and the profiler will automatically capture profiles for it.
```java
Span span = tracer.spanBuilder("findNearestVehicle").startSpan();
try (Scope s = span.makeCurrent()){
    // Your code goes here.
} finally {
    span.end();
}

// Your code goes here.
```

## View the span profiles in Grafana Tempo

To view the span profiles in Grafana Tempo, you need to have a Grafana instance running and a data source configured to link trace spans and profiles.

Refer to the [data source configuration documentation](/docs/grafana/datasources/tempo/configure-tempo-data-source) to see how to configure the visualization to link trace spans with profiles.

To use a simple configuration, follow these steps:

1. Select a Pyroscope data source from the Data source drop-down.

2. Optional: Choose any tags to use in the query. If left blank, the default values of service.name and service.namespace are used.

The tags you configure must be present in the spans attributes or resources for a trace to profiles span link to appear. You can optionally configure a new name for the tag. This is useful for example if the tag has dots in the name and the target data source doesn’t allow using dots in labels. In that case you can for example remap service.name to service_name.

3. Select one or more profile types to use in the query. Select the drop-down and choose options from the menu.

The profile type or app must be selected for the query to be valid. Grafana doesn’t show any data if the profile type or app isn’t selected when a query runs.

![span-profiles configuration](https://grafana.com/static/img/docs/tempo/profiles/Tempo-data-source-profiles-Settings.png)

## Examples

Check out the [examples](https://github.com/grafana/pyroscope/tree/main/examples/tracing/tempo) directory for a complete demo application that shows tracing integration features.