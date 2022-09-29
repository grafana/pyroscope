---
aliases:
  - /docs/fire/latest/operators-guide/configuring/configuring-tracing/
description: Learn how to configure Grafana Fire to send traces to Jaeger.
menuTitle: Configuring tracing
title: Configuring Grafana Fire tracing
weight: 80
---

# Configuring Grafana Fire tracing

Grafana Fire uses [Jaeger](https://www.jaegertracing.io/) to implement distributed
tracing. Jaeger is a valuable tool for troubleshooting the behavior of
Grafana Fire in production.

## Dependencies

Set up Jaeger deployment to collect and store traces from Grafana Fire. A
deployment includes either the Jaeger all-in-one binary, or a distributed
system of agents, collectors, and queriers. If you run Grafana Fire on Kubernetes, refer to [Jaeger
Kubernetes](https://github.com/jaegertracing/jaeger-kubernetes).

## Configuration

To configure Grafana Fire to send traces, perform the following steps:

1. Set the `JAEGER_AGENT_HOST` environment variable in all components to point
   to the Jaeger agent.
1. Enable sampling in the appropriate components:
   - The ingester and ruler self-initiate traces and should have sampling
     explicitly enabled.
   - Sampling for the distributor and query-frontend can be enabled in Grafana Fire
     or in an upstream service, like a proxy or gateway running in front of Grafana Fire.

To enable sampling in Grafana Fire components you can specify either
`JAEGER_SAMPLER_MANAGER_HOST_PORT` for remote sampling, or
`JAEGER_SAMPLER_TYPE` and `JAEGER_SAMPLER_PARAM` to manually set sampling
configuration. Refer to [Jaeger Client Go
documentation](https://github.com/jaegertracing/jaeger-client-go#environment-variables)
for the full list of environment variables you can configure.

Note that you must specify one of `JAEGER_AGENT_HOST` or
`JAEGER_SAMPLER_MANAGER_HOST_PORT` in each component for Jaeger to be enabled,
even if you plan to use the default values.
