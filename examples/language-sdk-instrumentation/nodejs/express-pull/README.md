# Pyroscope pull mode with Grafana Alloy

This example demonstrates how Pyroscope can be used to scrape pprof profiles from remote nodejs targets using [Grafana Alloy](https://grafana.com/docs/alloy/latest/).

Instead of the application pushing profiles to Pyroscope, Alloy periodically scrapes the pprof endpoints exposed by each nodejs instance and forwards the profiles to the Pyroscope server.

### 1. Run Pyroscope server, Alloy, and demo application in docker containers

```shell
docker-compose up -d
```

As a sample application we use a slightly modified rideshare app, started in three regions (`us-east`, `eu-north`, `ap-south`). Each instance exposes pprof endpoints via the `@pyroscope/nodejs` express middleware.

The scrape configuration lives in [`alloy.config.alloy`](./alloy.config.alloy): Alloy scrapes each target and forwards the profiles to the Pyroscope `write` endpoint.

### 2. Observe profiling data

Profiling is more fun when the application does some work, so it ships with a built-in load generator.

Now that everything is set up, you can browse profiling data via the [Pyroscope UI](http://localhost:4040) or the bundled [Grafana instance](http://localhost:3000). Alloy's own UI is available at http://localhost:12345.
