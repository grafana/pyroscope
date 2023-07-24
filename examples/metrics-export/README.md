# Exporting Metrics

Pyroscope Server offers a way to export values of a particular stack trace sample from your profiles as a Prometheus
metric. You can find more details on how to use exporting in our [documentation](http://pyroscope.io/docs/metrics-export).

The directory contains example setup consisting of:
 - Two instances of sample Go application: one with `env=prod` tag and another one with `env=staging`.
 - [Pyroscope Server](http://localhost:4040)
 - [Prometheus](http://localhost:9090)
 - [Grafana](http://localhost:3000)

In the example, Pyroscope Server is configured to export sampled values gathered with pyroscope agent:

```yaml
metrics-export-rules:

  # The metrics is a sum of all observed CPU samples of 'slowFunction'
  # of production app instance.
  app_slow_function_prod_cpu_seconds_total:
    expr: simple.golang.app.cpu{env="prod"}
    node: slowFunction

  # The metrics is a sum of all observed CPU samples of 'slowFunction'
  # with break down by 'env' tag.
  app_slow_function_env_cpu_seconds_total:
    expr: simple.golang.app.cpu
    node: slowFunction
    group_by: [ env ]

  # The metrics below are listed for demonstration purpose.
  # It's better to collect them via runtime instrumentation,
  # for example, using Prometheus.

  app_cpu_seconds_total:
    expr: simple.golang.app.cpu

  app_alloc_space_bytes:
    expr: simple.golang.app.alloc_space

  app_alloc_objects_total:
    expr: simple.golang.app.alloc_objects

  app_inuse_space_bytes:
    expr: simple.golang.app.inuse_space

  app_inuse_objects_total:
    expr: simple.golang.app.inuse_objects

```

To run the example execute the following command:

```shell
docker-compose up
```

Now you should be able to open sample Grafana [dashboard](http://localhost:3000) with exported metrics:

![image](https://user-images.githubusercontent.com/12090599/137328239-a56caea7-496b-4997-9a65-e7878597f83b.png)

