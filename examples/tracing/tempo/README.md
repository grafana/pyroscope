# Span Profiles with Grafana Tempo and Pyroscope

The docker compose consists of:
 - Ride share demo application
 - Tempo
 - Pyroscope
 - Grafana

`rideshare` applications generate traces and profiling data that should be available in Grafana.
Pyroscope and Tempo datasources are provisioned automatically.

### Build and run

The project can be run locally with the following commands:

```shell
docker-compose up
```

Navigate to the [Explore page](http://localhost:3000/explore?schemaVersion=1&panes=%7B%22yM9%22:%7B%22datasource%22:%22tempo%22,%22queries%22:%5B%7B%22refId%22:%22A%22,%22datasource%22:%7B%22type%22:%22tempo%22,%22uid%22:%22tempo%22%7D,%22queryType%22:%22traceqlSearch%22,%22limit%22:20,%22tableType%22:%22traces%22,%22filters%22:%5B%7B%22id%22:%22e73a615e%22,%22operator%22:%22%3D%22,%22scope%22:%22span%22%7D,%7B%22id%22:%22service-name%22,%22tag%22:%22service.name%22,%22operator%22:%22%3D%22,%22scope%22:%22resource%22,%22value%22:%5B%22ride-sharing-app%22%5D,%22valueType%22:%22string%22%7D%5D%7D%5D,%22range%22:%7B%22from%22:%22now-6h%22,%22to%22:%22now%22%7D%7D%7D&orgId=1), select a trace and click on one of its spans
that have a linked profile:

![image](https://github.com/grafana/otel-profiling-go/assets/12090599/31e33cd1-818b-4116-b952-c9ec7b1fb593)

By default, only the root span gets labeled (the first span created locally): such spans are marked with the _link_ icon
and have `pyroscope.profile.id` attribute set to the corresponding span ID.
Please note that presence of the attribute does not necessarily
indicate that the span has a profile: stack trace samples might not be collected, if the utilized CPU time is
less than the sample interval (10ms).

### Instrumentation

 - `rideshare` demo application instrumented with OpenTelemetry:
   - Go [OTel integration](https://github.com/grafana/otel-profiling-go)
   - Java [OTel integration](https://github.com/grafana/otel-profiling-java) 
 - `pyroscope` itself is instrumented with `opentracing-go` SDK and [`spanprofiler`](../../../pkg/util/spanprofiler) for profiling integration.

### Grafana Tempo configuration

In order to correlate trace spans with profiling data, Tempo datasource should be configured:
 - Data source of the profiling data.
 - Tags to use in the query.
 - Profile type: as of now, only CPU time profile is fully supported.
 - Query override.

![image](https://github.com/grafana/pyroscope/assets/12090599/380ac574-a298-440d-acfb-7bc0935a3a7c)

While tags are optional, configuring them is highly recommended for optimizing query performance.
In our example, we configured the `host.name` tag for use in Pyroscope queries as the `hostname` label.
This configuration restricts the data set for lookup to a specific host, ensuring that queries remain
consistently fast. Note that the tags you configure must be present in the spans attributes or resources
for a trace to profiles span link to appear.

Please refer to our [documentation](https://grafana.com/docs/grafana/next/datasources/tempo/configure-tempo-data-source/#trace-to-profiles) for more details.
