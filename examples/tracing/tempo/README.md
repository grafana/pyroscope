### This is a pre-release demo project for internal use only

The docker compose consists of:
 - `rideshare` demo application instrumented with OpenTelemetry and [Pyroscope SDK](https://github.com/grafana/otel-profiling-go)
 - Tempo
 - Pyroscope
 - Grafana

`rideshare` applications generate traces and profiling data that should be available in Grafana.
Pyroscope and Tempo datasources are provisioned automatically.

### Build and run

The project can be run locally with the following commands:

```shell
docker-compose up --build
```

Navigate to the [Explore page](http://localhost:3000/explore?schemaVersion=1&panes=%7B%22yM9%22:%7B%22datasource%22:%22tempo%22,%22queries%22:%5B%7B%22refId%22:%22A%22,%22datasource%22:%7B%22type%22:%22tempo%22,%22uid%22:%22tempo%22%7D,%22queryType%22:%22traceqlSearch%22,%22limit%22:20,%22tableType%22:%22traces%22,%22filters%22:%5B%7B%22id%22:%22e73a615e%22,%22operator%22:%22%3D%22,%22scope%22:%22span%22%7D,%7B%22id%22:%22service-name%22,%22tag%22:%22service.name%22,%22operator%22:%22%3D%22,%22scope%22:%22resource%22,%22value%22:%5B%22ride-sharing-app%22%5D,%22valueType%22:%22string%22%7D%5D%7D%5D,%22range%22:%7B%22from%22:%22now-6h%22,%22to%22:%22now%22%7D%7D%7D&orgId=1), select a trace and click on one of its spans
that have a linked profile:

![image](https://github.com/grafana/otel-profiling-go/assets/12090599/31e33cd1-818b-4116-b952-c9ec7b1fb593)

By default, only the root span gets labeled (the first span created locally): such spans are marked with the
`pyroscope.profile.id` attribute set to the span ID. Please note that presence of the attribute does not necessarily
indicate that the span has a profile: stack trace samples might not be collected, if the utilized CPU time is
less than the sample interval (10ms).
