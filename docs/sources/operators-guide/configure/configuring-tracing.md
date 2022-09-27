---
aliases:
  - /docs/mimir/latest/operators-guide/configuring/configuring-tracing/
description: Learn how to configure Grafana Mimir to send traces to Jaeger.
menuTitle: Configuring tracing
title: Configuring Grafana Mimir tracing
weight: 100
---

# Configuring Grafana Mimir tracing

Grafana Mimir uses [Jaeger](https://www.jaegertracing.io/) to implement distributed
tracing. Jaeger is a valuable tool for troubleshooting the behavior of
Grafana Mimir in production.

## Dependencies

Set up Jaeger deployment to collect and store traces from Grafana Mimir. A
deployment includes either the Jaeger all-in-one binary, or a distributed
system of agents, collectors, and queriers. If you run Grafana Mimir on Kubernetes, refer to [Jaeger
Kubernetes](https://github.com/jaegertracing/jaeger-kubernetes).

## Configuration

To configure Grafana Mimir to send traces, perform the following steps:

1. Set the `JAEGER_AGENT_HOST` environment variable in all components to point
   to the Jaeger agent.
1. Enable sampling in the appropriate components:
   - The ingester and ruler self-initiate traces and should have sampling
     explicitly enabled.
   - Sampling for the distributor and query-frontend can be enabled in Grafana Mimir
     or in an upstream service, like a proxy or gateway running in front of Grafana Mimir.

To enable sampling in Grafana Mimir components you can specify either
`JAEGER_SAMPLER_MANAGER_HOST_PORT` for remote sampling, or
`JAEGER_SAMPLER_TYPE` and `JAEGER_SAMPLER_PARAM` to manually set sampling
configuration. Refer to [Jaeger Client Go
documentation](https://github.com/jaegertracing/jaeger-client-go#environment-variables)
for the full list of environment variables you can configure.

Note that you must specify one of `JAEGER_AGENT_HOST` or
`JAEGER_SAMPLER_MANAGER_HOST_PORT` in each component for Jaeger to be enabled,
even if you plan to use the default values.
