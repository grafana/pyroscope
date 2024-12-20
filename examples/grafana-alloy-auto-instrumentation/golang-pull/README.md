# Grafana Alloy Pull Mode Integration

This example demonstrates how you can use Grafana Alloy with Grafana Pyroscope (formerly known as Phlare).

### 1. Configure Grafana Alloy

You'll need to configure the Grafana Alloy for things like profiling configuration, targets, and possibly authentication in order to have the Grafana Alloy pull profiles from your application.

You can find a list of [arguments](https://grafana.com/docs/alloy/latest/reference/components/pyroscope/pyroscope.scrape/#arguments) and [supported blocks](https://grafana.com/docs/alloy/latest/reference/components/pyroscope/pyroscope.scrape/#blocks) in the [Grafana Alloy documentation for pyroscope](https://grafana.com/docs/alloy/latest/reference/components/pyroscope/pyroscope.scrape/)

Refer to [config file](alloy/config.alloy) to see an example of how to configure Grafana Alloy to send profiling data to Pyroscope.

### 2. Run Grafana Alloy, Grafana and Pyroscope

```shell
# Pull latest pyroscope and grafana images:
docker pull grafana/pyroscope:latest
docker pull grafana/grafana:latest
docker pull grafana/alloy:latest

docker-compose up -d
```

### 3. Observe profiling data

Now that everything is set up, you can browse profiling data.

#### Explore profiles
[Explore profiles app](http://localhost:3000/a/grafana-pyroscope-app/profiles-explorer).

![image](https://github.com/user-attachments/assets/71cb5a6e-2f5f-4f80-b868-d17fc30c2ca1)
![image](https://github.com/user-attachments/assets/00e45eac-0d2d-4229-85f0-3d2321c4542a)

#### Dashboard
You will also find a dummy [dashboard](http://localhost:3000/d/65gjqY3Mk/main).

