---
title: "Grafana Alloy"
menuTitle: "Grafana Alloy"
description: "Send data from your application using Grafana Alloy."
weight: 200
aliases:
  - /docs/phlare/latest/configure-client/grafana-agent/
  - ./grafana-agent # /docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/grafana-agent/
---

# Grafana Alloy

You can send data from your application using Grafana Alloy (preferred) or Grafana Agent (legacy) collectors.
Both collectors support profiling with eBPF, Java, and Golang in pull mode.

[//]: # 'Shared content for supported languages with eBPF'
[//]: # 'This content is located in /pyroscope/docs/sources/shared/supported-languages-ebpf.md'

{{< docs/shared source="pyroscope" lookup="supported-languages-ebpf.md" version="latest" >}}

[Grafana Alloy](https://grafana.com/docs/alloy/<ALLOY_VERSION>/) is a vendor-neutral distribution of the OpenTelemetry (OTel) Collector.
Alloy uniquely combines the very best OSS observability signals in the community.
Alloy uses configuration files written in Alloy configuration syntax.
For  more information, refer to the [Alloy configuration syntax](https://grafana.com/docs/alloy/<ALLOY_VERSION>/get-started/configuration-syntax/).

Alloy is the recommended collector instead of Grafana Agent.
New installations should use Alloy.

The instructions in this section explain how to use Alloy.

{{< admonition type="note" >}}
Refer to [Available profiling types](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/profile-types/) for a list of supported profile types.
{{< /admonition >}}

## Legacy collector, Grafana Agent

{{< docs/shared lookup="agent-deprecation.md" source="alloy" version="next" >}}

Grafana Agent is a legacy tool for collecting and forwarding profiling data.
Agent supports for eBPF and Golang in pull mode.
For information about Agent, refer to [Grafana Agent Flow](https://grafana.com/docs/agent/<AGENT_VERSION>/flow/).

Instructions for using Grafana Agent are available in documentation for Pyroscope v1.8 and earlier.

## eBPF profiling

eBPF (Extended Berkeley Packet Filter) is a modern Linux kernel technology that allows for safe, efficient, and customizable tracing of system and application behaviors without modifying the source code or restarting processes.

Benefits of eBPF profiling:

- Low overhead: eBPF collects data with minimal impact on performance.
- Versatile: eBPF can trace system calls, network packets, and even user-space application logic.
- Dynamic: No need to recompile or restart applications. eBPF allows for live tracing.

### Set up eBPF profiling

1. Ensure your system runs a Linux kernel version 4.9 or newer.
1. Install a collector, such as Alloy, on the target machine or container.
1. Configure Alloy to use eBPF for profiling. Refer to the [eBPF documentation](/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/grafana-alloy/ebpf) for detailed steps.
1. The collector collects eBPF profiles and sends them to the Pyroscope server.

### Supported languages

[//]: # 'Shared content for supported languages with eBPF'
[//]: # 'This content is located in /pyroscope/docs/sources/shared/supported-languages-ebpf.md'

{{< docs/shared source="pyroscope" lookup="supported-languages-ebpf.md" version="latest" >}}

## Golang profiling in pull mode

In pull mode, the collector periodically retrieves profiles from Golang applications, specifically targeting the pprof endpoints.

### Benefits of Golang profiling in pull mode

- Non-intrusive: No need to modify your application’s source code.
- Centralized profiling: Suitable for environments with multiple Golang applications or microservices.
- Automatic: Alloy handles the pulling and sending of profiles, requiring minimal configuration.

### Set up Golang profiling in pull mode

1. Ensure your Golang application exposes pprof endpoints.
1. Install and configure Alloy on the same machine or container where your application runs.
1. Ensure Alloy is set to pull mode and targeting the correct pprof endpoints. For step-by-step instructions, visit the [Go (Pull Mode)](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/grafana-alloy/go_pull) documentation.
1. The collector queries the pprof endpoints of your Golang application, collects the profiles, and forwards them to the Pyroscope server.

## Receive profiles from Pyroscope SDKs

Alloy can receive profiles from applications instrumented with Pyroscope SDKs through the `pyroscope.receive_http` component. This approach provides several key advantages:
- Improved performance by sending profiles to a local Alloy instance instead of over the internet to Grafana Cloud, reducing latency and application impact
- Separation of infrastructure concerns from application code - developers don't need to handle authentication, tenant configuration, or infrastructure labels in their code
- Centralized management of authentication and metadata enrichment (for example, Kubernetes labels, business labels)

This capability is not available in the legacy Grafana Agent.

### Set up profile receiving

1. Configure your application with a Pyroscope SDK pointing to the `pyroscope.receive_http` Alloy component.
2. For step-by-step instructions, refer to the [Receive profiles from Pyroscope SDKs](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/grafana-alloy/receive_profiles) documentation.

## Next steps

Whether using eBPF for versatile system and application profiling or relying on Golang's built-in pprof endpoints in pull mode, Alloy collectors offer streamlined processes to gather essential profiling data.
Choose the method that best fits your application and infrastructure needs.

[troubleshooting]: /docs/alloy/<ALLOY_VERSION>/reference/components/pyroscope/pyroscope.ebpf/#troubleshooting-unknown-symbols
