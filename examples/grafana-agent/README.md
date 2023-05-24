# Grafana Agent Pull Mode Integration

This example demonstrates how you can use Grafana Agent with Grafana Pyroscope (formerly known as Phlare).

### 1. Run Grafana agent, Grafana and Pyroscope

```shell
docker-compose up -d
```

### 2. Observe profiling data

Now that everything is set up, you can browse profiling data via [Grafana UI](http://localhost:3000).

### Configuration

Refer to [config file](./agent/config.river) to see how to configure Grafana Agent to send profiling data to Pyroscope.
