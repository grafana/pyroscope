# Span Profiles with Grafana Tempo and Pyroscope

The docker compose consists of:
- Three Ruby Rideshare App instances (us-east, eu-north, ap-south regions)
- Tempo for trace collection
- Pyroscope for continuous profiling
- Grafana for visualization
- Load Generator for simulating traffic

For a detailed guide about Ruby span profiles configuration, refer to the docs [Pyroscope documentation](https://grafana.com/docs/pyroscope/latest/configure-client/trace-span-profiles/ruby-span-profiles/).

The `rideshare` app generates traces and profiling data that should be available in Grafana.
Pyroscope and Tempo datasources are provisioned automatically.

### Build and run

The project can be run locally with the following commands:

```shell
# Pull latest pyroscope and grafana images:
docker pull grafana/pyroscope:latest
docker pull grafana/grafana:latest

bundle install

docker compose up
```
The load generator will automatically start sending requests to all regional instances.

### Viewing Traces and Profiles

Navigate to the [Explore page](http://localhost:3000/explore?schemaVersion=1&panes=%7B%22yM9%22:%7B%22datasource%22:%22tempo%22,%22queries%22:%5B%7B%22refId%22:%22A%22,%22datasource%22:%7B%22type%22:%22tempo%22,%22uid%22:%22tempo%22%7D,%22queryType%22:%22traceqlSearch%22,%22limit%22:20,%22tableType%22:%22traces%22,%22filters%22:%5B%7B%22id%22:%22e73a615e%22,%22operator%22:%22%3D%22,%22scope%22:%22span%22%7D,%7B%22id%22:%22service-name%22,%22tag%22:%22service.name%22,%22operator%22:%22%3D%22,%22scope%22:%22resource%22,%22value%22:%5B%22rideshare.ruby.push.app%22%5D,%22valueType%22:%22string%22%7D%5D%7D%5D,%22range%22:%7B%22from%22:%22now-6h%22,%22to%22:%22now%22%7D%7D%7D&orgId=1), select a trace and click on a span that has a linked profile:

![image](https://github.com/grafana/otel-profiling-go/assets/12090599/31e33cd1-818b-4116-b952-c9ec7b1fb593)

By default, only the root span gets labeled (the first span created locally): such spans are marked with the link icon
and have `pyroscope.profile.id` attribute set to the corresponding span ID.
Please note that presence of the attribute does not necessarily
indicate that the span has a profile: stack trace samples might not be collected, if the utilized CPU time is
less than the sample interval (10ms).

### Grafana Tempo configuration

In order to correlate trace spans with profiling data, the Tempo datasource should have the following configured:
- The profiling data source
- Tags to use when making profiling queries

![image](https://github.com/grafana/pyroscope/assets/12090599/380ac574-a298-440d-acfb-7bc0935a3a7c)

While tags are optional, configuring them is highly recommended for optimizing query performance.
In our example, we configured the `service.name` tag for use in Pyroscope queries as the `service_name` label.
This configuration restricts the data set for lookup, ensuring that queries remain
consistently fast. Note that the tags you configure must be present in the span attributes or resources
for a trace to profiles span link to appear.

Please refer to our [documentation](https://grafana.com/docs/grafana/next/datasources/tempo/configure-tempo-data-source/#trace-to-profiles) for more details.
