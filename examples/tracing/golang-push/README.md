# Span profiles with Traces to profiles for Go

This example consists of:
- The Go Rideshare App
- Tempo
- Pyroscope
- Grafana

The `rideshare` app generate traces and profiling data that then can be
analysed in Grafana. The datasources for Pyroscope and Tempo are provisioned
automatically.

## Usage

The project can be run locally with the following commands:

```shell
# Pull latest pyroscope and grafana images:
docker pull grafana/pyroscope:latest
docker pull grafana/grafana:latest

docker compose up
```

Using the [Explore Profiles app], you can inspect the profiles for different request types:


[Explore Profiles app]:http://localhost:3000/a/grafana-pyroscope-app/profiles-explorer?searchText=&panelType=time-series&layout=grid&hideNoData=off&explorationType=labels&var-serviceName=ride-sharing-app&var-profileMetricId=process_cpu:cpu:nanoseconds:cpu:nanoseconds&var-dataSource=pyroscope&var-groupBy=all&var-filters=

![Explore Profiles screenshot](https://github.com/user-attachments/assets/6e6f1b35-4494-4f8f-afba-b231b09d4565)


Navigate to the [Explore Tempo page], select a trace and click on a span that has a linked profile:

[Explore Tempo page]: http://localhost:3000/explore?schemaVersion=1&panes=%7B%22yM9%22:%7B%22datasource%22:%22tempo%22,%22queries%22:%5B%7B%22refId%22:%22A%22,%22datasource%22:%7B%22type%22:%22tempo%22,%22uid%22:%22tempo%22%7D,%22queryType%22:%22traceqlSearch%22,%22limit%22:20,%22tableType%22:%22traces%22,%22filters%22:%5B%7B%22id%22:%22e73a615e%22,%22operator%22:%22%3D%22,%22scope%22:%22span%22%7D,%7B%22id%22:%22service-name%22,%22tag%22:%22service.name%22,%22operator%22:%22%3D%22,%22scope%22:%22resource%22,%22value%22:%5B%22ride-sharing-app%22%5D,%22valueType%22:%22string%22%7D%5D%7D%5D,%22range%22:%7B%22from%22:%22now-6h%22,%22to%22:%22now%22%7D%7D%7D&orgId=1

![image](https://github.com/grafana/otel-profiling-go/assets/12090599/31e33cd1-818b-4116-b952-c9ec7b1fb593)

By default, only the root span gets labeled (the first span created locally): such spans are marked with the _link_ icon
and have `pyroscope.profile.id` attribute set to the corresponding span ID.
Please note that presence of the attribute does not necessarily
indicate that the span has a profile: stack trace samples might not be collected, if the utilized CPU time is
less than the sample interval (10ms).


### Instrumentation

- `rideshare` demo application instrumented with OpenTelemetry: [OTel integration] . Please refer to our [documentation] for more details.
- `pyroscope` itself is instrumented with `opentracing-go` SDK and [`spanprofiler`] for profiling integration.

[OTel integration]:https://github.com/grafana/otel-profiling-go
[`spanprofiler`]:https://github.com/grafana/dskit/tree/main/spanprofiler
[documentation]:https://grafana.com/docs/pyroscope/latest/configure-client/trace-span-profiles/go-span-profiles/


### Grafana Tempo configuration

Please refer to our [documentation](https://grafana.com/docs/grafana/next/datasources/tempo/configure-tempo-data-source/#trace-to-profiles) for more details.
