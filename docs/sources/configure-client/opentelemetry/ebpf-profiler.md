---
title: OpenTelemetry Profiles Support (Experimental)
menuTitle: OpenTelemetry Profiles
description: Pyroscope's experimental support for OpenTelemetry profiles
weight: 10
---

# OpenTelemetry profiles support (experimental)

Pyroscope now includes experimental support for receiving and visualizing profiles from OpenTelemetry sources. This integration allows you to:

- Collect system-wide profiles using the [OpenTelemetry eBPF profiler](https://github.com/open-telemetry/opentelemetry-ebpf-profiler)
- Process profile data through the OpenTelemetry Collector
- Visualize profiles in Grafana using Pyroscope

## Important Notes About Current Status

Before getting started, please be aware of the following limitations:

- The OpenTelemetry profiles protocol ([proto files](https://github.com/open-telemetry/opentelemetry-proto/tree/main/opentelemetry/proto/profiles)) is under active development:
    - Breaking changes are expected and have occurred
    - Compatibility between components (profiler, collector, backend) requires careful version management
    - We maintain support for the latest protocol version, but updates may be required frequently

- Symbolization support is currently limited:
  - Function names may not appear in flamegraphs for some programs
  - We're working on improving symbol resolution and adding support for manual symbol uploads

- We recommend evaluating this feature for development and testing purposes, but waiting for protocol stabilization before production use

## Requirements

- Linux system (amd64/arm64) for eBPF profiler
- OpenTelemetry Collector with profiles feature gate enabled
- Grafana with Pyroscope data source enabled

## Architecture

The profile collection pipeline consists of:

1. **OpenTelemetry eBPF Profiler**: Collects system-wide profiles
2. **OpenTelemetry Collector**: Receives and forwards profile data
3. **Pyroscope**: Stores and processes profiles
4. **Grafana**: Visualizes profile data

## Getting Started

For detailed setup instructions and working examples, refer to our [examples repository](https://github.com/grafana/pyroscope/tree/main/examples/grafana-agent-auto-instrumentation/ebpf-otel).

The examples demonstrate deployments for both Docker and Kubernetes environments.
