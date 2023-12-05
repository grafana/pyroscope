## OTel Tracing integration example

**NOTE: This example is just a Proof-of-concept at the moment. There is ongoing efforts to create an offical opentelemetry profiling signal and this example will be update to reflect that once it is approved**

Currently this integration is supported for:
- Go: [OTel Profiling Go](https://github.com/pyroscope-io/otel-profiling-go)
- Java: [OTel Profiling Java](https://github.com/pyroscope-io/otel-profiling-java)

The example demonstrates how Pyroscope can be used in conjunction with tracing.
In the example we will instrument a sample application with OpenTelemetry tracer and
will be using [Jaeger](https://www.jaegertracing.io) and [Grafana](https://grafana.com).

To achieve that, we will be using a special label – `profile_id` that is set by the profiler
dynamically. Our simple application specifies Span ID as a profile ID which establishes
one-to-one relation between a trace span execution scope and the profiling scope. By default, only the root span gets annotated (the first span created locally), this is done to circumvent the fact that the profiler records only the time spent on CPU.

There are a few limitations:

1.  Only Go CPU profiling is fully supported at the moment.
2.  Due to the very idea of the sampling profilers, spans shorter than the sample interval may
    not be captured. For example, Go CPU profiler probes stack traces 100 times per second,
    meaning that spans shorter than 10ms may not be captured.

### 1. Run the docker-compose file

```
# You will need to add loki plugin as well for this example
docker plugin install grafana/loki-docker-driver:latest --alias loki --grant-all-permissions
docker-compose up --build
```

Optionally, for debugging purposes, you can specify `DEBUG_TRACER` variable to make the sample app
printing traces to stdout instead of sending them to Jaeger:

```
# DEBUG_TRACER=1 docker-compose up --build
```

### 2. Access profile exemplars via Jaeger UI

The newly collected data should be available for querying. Open [Jaeger UI](http://localhost:4000) and query a trace:

![image](https://user-images.githubusercontent.com/23323466/162067415-07737db7-9978-4f2b-a99a-bc9b7a0faa66.png)

### 3. Access profile exemplars via Jaeger in Grafana

The newly collected data should be available for querying. Open [Grafana](http://localhost:3000)
and Navigate to the **Explore** page and query a trace:

[![Watch the video](https://user-images.githubusercontent.com/23323466/172881613-842f67f0-6bfa-4671-a44a-e966d5ca67a4.mov)](https://user-images.githubusercontent.com/23323466/172881613-842f67f0-6bfa-4671-a44a-e966d5ca67a4.mov)

### 4. Access profiling data via Pyroscope UI

Now let's filter out spans with the `pyroscope.profile.id` attribute. It's also important to note
that only **root** spans have profiles: in our case these are `OrderVehicle` and `CarHandler`:

![image](https://user-images.githubusercontent.com/12090599/153310051-4f7b9fd2-ae9b-4e61-9714-7fb8c71a331f.png)

Click on the `pyroscope.profile.url` tag value to open [Pyroscope UI](http://localhost:4040) with
the span CPU time flamegraph:

![image](https://user-images.githubusercontent.com/12090599/153314565-c7be8ef6-cd5d-4d0b-9070-83ae8a3a8e8a.png)
