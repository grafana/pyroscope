# Grafana Agent Java Profiling via Auto-instrumentation Example

This repository provides a practical demonstration of leveraging the Grafana Agent for continuous Java application profiling using Pyroscope in a dockerized environment. It illustrates a seamless approach to profiling Java processes, aiding in performance optimization.

## Overview

The example illustrates the utilization of Grafana Agent for continuous Java process profiling. It employs Pyroscope with the async-profiler to facilitate this, offering an insightful view into application performance.

### Profiling Methodologies

Java applications can be profiled via Pyroscope using three methodologies, The example emphasizes the first method, showcasing the attachment of the Pyroscope profiler as a `javaagent`. 

Refer to the [official documentation](https://grafana.com/docs/pyroscope/latest/configure-client/grafana-agent/java/) for an in-depth understanding and additional configuration options for Java profiling with the Grafana Agent.

The three methodologies are as follows:

1. **Java Agent (this example): Attach the Pyroscope profiler as a `javaagent` to a running process, initializing the profiler at application startup without source code modifications or extra dependencies. Configuration is manageable via Java parameters or environment variables.**

2. **OTel Integration ([docs](https://grafana.com/docs/pyroscope/next/configure-client/trace-span-profiles/java-span-profiles/)):** Utilize the OTel integration as an extension to `otel-java-instrumentation`, in conjunction with `opentelemetry-javaagent` to link java profiles with trace spans. This method is suitable for applications already using OpenTelemetry.

3. **Direct Dependency ([docs](https://grafana.com/docs/pyroscope/next/configure-client/language-sdks/java/)):** Integrate Pyroscope directly within your application's code, allowing dynamic profiler management.


### Grafana Agent and Async-Profiler

The Grafana Agent automates Java process discovery for profiling, streamlining the setup per application. It enables precise and targeted profiling configurations through the Grafana Agent settings.

## Getting Started

To use this example:

1. Install and run Docker.
2. Clone this repository and navigate to the example's directory.
3. Use Docker Compose to build and initiate the container:

```shell
   docker-compose up --build
```

The Grafana Agent starts profiling the Java application as per the defined configuration once the container is operational.

## Considerations
Root privileges are necessary for running the Grafana Agent for profiling, which must be executed within the host's PID namespace.

## Documentation
Refer to the [official documentation](https://grafana.com/docs/pyroscope/latest/configure-client/grafana-agent/java/) for an in-depth understanding and additional configuration options for Java profiling with the Grafana Agent.