# Grafana Agent Java profiling via auto-instrumentation example

This repository provides a practical demonstration of leveraging the Grafana Agent for continuous Java application profiling using Pyroscope in a dockerized environment. It illustrates a seamless approach to profiling Java processes, aiding in performance optimization.

## Overview

Using this example, you'll learn how to use Grafana Agent for continuous Java process profiling using Pyroscope with the async-profiler. This example provides an insightful view into application performance.

### Profiling methodologies

Java applications can be profiled via Pyroscope using three methodologies. The example showcases the the Pyroscope profiler as a `javaagent` to a running process, initializing the profiler at application startup without source code modifications or extra dependencies. Manage configuration using Java parameters or environment variables.

Refer to the [official documentation](https://grafana.com/docs/pyroscope/latest/configure-client/grafana-agent/java/) for an in-depth understanding and additional configuration options for Java profiling with the Grafana Agent.

### Grafana Agent and async-profiler

The Grafana Agent automates Java process discovery for profiling, streamlining the setup per application. It enables precise and targeted profiling configurations through the Grafana Agent settings.

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