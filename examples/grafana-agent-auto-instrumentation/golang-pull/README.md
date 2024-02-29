# Grafana Agent Pull Mode Integration

This example demonstrates how you can use Grafana Agent with Grafana Pyroscope (formerly known as Phlare).

### 1. Configure Grafana agent

You'll need to configure the Grafana agent for things like profiling configuration, targets, and possibly authentication in order to have the Grafana Agent pull profiles from your application.

You can find a list of [arguments](https://grafana.com/docs/agent/latest/flow/reference/components/pyroscope.scrape/#arguments) and [supported blocks](https://grafana.com/docs/agent/latest/flow/reference/components/pyroscope.scrape/#blocks) in the [Grafana Agent documentation for pyroscope](https://grafana.com/docs/agent/latest/flow/reference/components/pyroscope.scrape/)

Refer to [config file](agent/config/config.river) to see an example of how to configure Grafana Agent to send profiling data to Pyroscope.

### 2. Run Grafana agent, Grafana and Pyroscope

```shell
docker-compose up -d
```

### 3. Observe profiling data

Now that everything is set up, you can browse profiling data via [Grafana UI](http://localhost:3000).

#### Explore view
For showing profiling data alongside traces
![image](https://github.com/grafana/pyroscope/assets/23323466/a9c2f28c-d35a-49b0-a3bc-678d3fbdd321)

#### Dashboard
For showing real-time overview of profiling data
![image](https://github.com/grafana/pyroscope/assets/23323466/59a84d0c-87d2-4cfc-8e34-b54576cb6540)

