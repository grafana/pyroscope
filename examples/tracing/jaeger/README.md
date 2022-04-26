## WARNING: This feature is experimental

This is just an experimental feature and there are several improvements needed to make this
production ready. We would love to get feedback on how people view this feature and ideas on
how we could improve it for various use cases. Use at your own risk.

## Tracing integration example

The example demonstrates how Pyroscope can be used in conjunction with tracing.
In the example we will instrument a sample application with OpenTelemetry tracer and
will be using [Jaeger](https://www.jaegertracing.io) and [Grafana](https://grafana.com).

To achieve that, we will be using a special label – `profile_id` that is set by the profiler
dynamically. Our simple application specifies Span ID as a profile ID which establishes
one-to-one relation between a trace span execution scope and the profiling scope.

By default, only the root span gets annotated (the first span created locally), this is done to
circumvent the fact that the profiler records only the time spent on CPU. Otherwise, all the
children profiles should be merged to get the full representation of the root span profile.

In practice, the scenario can be different: an arbitrary string can be set as a profile ID.
The most important feature is that profiles with the same ID are merged by storage upon insert.

Pyroscope handles profiles with `profile_id` label in a very specific way and stores them
separately from others. As a result, such profiles can not be accessed using regular queries
that aggregate the data: the very idea of identifiers is to ensure request-level granularity.

There are number of limitations:

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

### 2. Access traces via Grafana.

The newly collected data should be available for querying. Open [Grafana](http://localhost:3000)
and Navigate to the **Explore** page and query a trace:

![image](https://user-images.githubusercontent.com/12090599/153313512-49dca6b8-7ccd-4483-a3a9-6ca88ee912c7.png)

![image](https://user-images.githubusercontent.com/12090599/153309817-0ed575fb-9219-4e11-8f11-63d89d0efde4.png)


### 3. Access traces via Jaeger UI.

The newly collected data should be available for querying. Open [Jaeger UI](http://localhost:4000) and query a trace:

![image](https://user-images.githubusercontent.com/23323466/162067415-07737db7-9978-4f2b-a99a-bc9b7a0faa66.png)


### 4. Access profiling data via Pyroscope UI.

Now let's filter out spans with the `pyroscope.profile.id` attribute. It's also important to note
that only **root** spans have profiles: in our case these are `OrderVehicle` and `CarHandler`:

![image](https://user-images.githubusercontent.com/12090599/153310015-ad3c9b21-14f4-41e3-a8a4-0bb569a65ca8.png)

![image](https://user-images.githubusercontent.com/12090599/153310051-4f7b9fd2-ae9b-4e61-9714-7fb8c71a331f.png)

Click on the `pyroscope.profile.url` tag value to open [Pyroscope UI](http://localhost:4040) with
the span CPU time flamegraph:

![image](https://user-images.githubusercontent.com/12090599/153314565-c7be8ef6-cd5d-4d0b-9070-83ae8a3a8e8a.png)
