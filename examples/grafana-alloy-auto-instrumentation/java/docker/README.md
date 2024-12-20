# Java profiling via auto-instrumentation example in Docker with Grafana Alloy

This repository provides a practical demonstration of leveraging the Grafana Alloy for continuous Java application profiling using Pyroscope in a dockerized environment. It illustrates a seamless approach to profiling Java processes, aiding in performance optimization.

## Overview

Grafana Alloy automates Java process discovery for profiling, streamlining the setup per application. It enables precise and targeted profiling configurations through Grafana Alloy settings.

Java profiling via Grafana Alloy is based on a few Grafana Alloy components:
- `discovery.process` (and optionally `discovery.kubernetes`) for process discovery
- `discovery.relabel` for detecting java processes and setting up labels
- `pyroscope.java` for enabling profiling for specific applications
- `pyroscope.write` for writing the profiles data to a remote endpoint

Refer to the [official documentation](https://grafana.com/docs/pyroscope/latest/configure-client/grafana-alloy/java/) for an in-depth understanding and additional configuration options for Java profiling with Grafana Alloy.
Also, check the [Grafana Alloy Components reference](https://grafana.com/docs/alloy/latest/reference/components/) for more details on each used component.

### async-profiler

The `pyroscope.java` component internally uses the [async-profiler](https://github.com/async-profiler/async-profiler) library.
This approach offers a key advantage over other instrumentation mechanisms in that you can profile applications that are already running without interruptions (code changes, config changes or restarts).

Under the hood, this is achieved by attaching to the application at a process level and issuing commands to control profiling.

## Getting started

To use this example:

1. Install and run Docker.
2. Clone this repository and navigate to the example's directory.
3. Use Docker Compose to build and initiate the container:

```shell
# Pull latest pyroscope and grafana images:
docker pull grafana/pyroscope:latest
docker pull grafana/grafana:latest
docker pull grafana/alloy:latest

docker-compose up --build
```

After the container is operational, Grafana Alloy profiles the Java application using he defined configuration.

### Observe profiling data

Now that everything is set up, you can browse profiling data through the [Explore profiles app](http://localhost:3000/a/grafana-pyroscope-app/profiles-explorer).

![image](https://github.com/user-attachments/assets/16f5559a-0bbc-4cf3-9589-fa4374bbc7e8)
![image](https://github.com/user-attachments/assets/ca28d228-93c3-4e16-a63c-285005c7b203)



## Considerations

You need root privileges to run Grafana Alloy for profiling. It must be executed within the host's PID namespace.

## Documentation

Refer to the [official documentation](https://grafana.com/docs/pyroscope/latest/configure-client/grafana-alloy/java/) for an in-depth understanding and additional configuration options for Java profiling with Grafana Alloy.
