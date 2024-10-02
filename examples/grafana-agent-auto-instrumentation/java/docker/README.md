# Grafana Agent Java profiling via auto-instrumentation example in Docker

This repository provides a practical demonstration of leveraging the Grafana Agent for continuous Java application profiling using Pyroscope in a dockerized environment. It illustrates a seamless approach to profiling Java processes, aiding in performance optimization.

## Overview

The Grafana Agent automates Java process discovery for profiling, streamlining the setup per application. It enables precise and targeted profiling configurations through the Grafana Agent settings.

Java profiling via the Grafana Agent is based on a few Grafana Agent components:
- `discovery.process` (and optionally `discovery.kubernetes`) for process discovery
- `discovery.relabel` for detecting java processes and setting up labels
- `pyroscope.java` for enabling profiling for specific applications
- `pyroscope.write` for writing the profiles data to a remote endpoint

Refer to the [official documentation](https://grafana.com/docs/pyroscope/latest/configure-client/grafana-agent/java/) for an in-depth understanding and additional configuration options for Java profiling with the Grafana Agent.
Also, check the [Grafana Agent Components reference](https://grafana.com/docs/agent/latest/flow/reference/components/) for more details on each used component.

### async-profiler

The `pyroscope.java` agent component internally uses the [async-profiler](https://github.com/async-profiler/async-profiler) library.
This approach offers a key advantage over other instrumentation mechanisms in that you can profile applications that are already running without interruptions (code changes, config changes or restarts).

Under the hood, this is achieved by attaching to the application at a process level and issuing commands to control profiling.

## Getting started

To use this example:

1. Install and run Docker.
2. Clone this repository and navigate to the example's directory.
3. Use Docker Compose to build and initiate the container:

```shell
   docker-compose up --build
```

After the container is operational, the Grafana Agent profiles the Java application using he defined configuration.

## Considerations

You need root privileges to run the Grafana Agent for profiling. The Agent must be executed within the host's PID namespace.

## Documentation

Refer to the [official documentation](https://grafana.com/docs/pyroscope/latest/configure-client/grafana-agent/java/) for an in-depth understanding and additional configuration options for Java profiling with the Grafana Agent.
