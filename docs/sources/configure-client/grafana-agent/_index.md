---
title: "Grafana Alloy and Grafana Agent"
menuTitle: "Grafana Alloy and Grafana Agent"
description: "Send data from your application via using Grafana Alloy or Grafana Agent."
weight: 10
aliases:
  - /docs/phlare/latest/configure-client/grafana-agent/
---

# Grafana Alloy and Grafana Agent

You can send data from your application using Grafana Alloy or Grafana Agent collectors.
Both collectors support profiling with eBPF, Java, and Golang in pull mode.

[Grafana Alloy](https://grafana.com/docs/alloy/latest/) is a vendor-neutral distribution of the OpenTelemetry (OTel) Collector.
Alloy uniquely combines the very best OSS observability signals in the community.
Grafana Alloy uses configuration file written using River.

Alloy is the recommended collector instead of Grafana Agent.
New installations should use Alloy.

{{< docs/shared lookup="agent-deprecation.md" source="alloy" version="next" >}}

Grafana Agent is a powerful tool for collecting and forwarding profiling data.
With the introduction of support for eBPF and continuing support for Golang in pull mode, Grafana Agent has become even more versatile in its capabilities.
This document provides an overview of these two modes of profiling and guides users on setting them up.

{{< admonition type="note" >}}
Refer to [Available profiling types]({{< relref "../../view-and-analyze-profile-data/profiling-types#available-profiling-types" >}}) for a list of profile types supported.
{{< /admonition >}}

## eBPF profiling

eBPF (Extended Berkeley Packet Filter) is a modern Linux kernel technology that allows for safe, efficient, and customizable tracing of system and application behaviors without modifying the source code or restarting processes.

Benefits of eBPF profiling:

- Low overhead: eBPF collects data with minimal impact on performance.
- Versatile: eBPF can trace system calls, network packets, and even user-space application logic.
- Dynamic: No need to recompile or restart applications. eBPF allows for live tracing.

### Set up eBPF profiling

1. Ensure your system runs a Linux kernel version 4.9 or newer.
1. Install a collector, such as Grafana Alloy (preferred) or Grafana Agent (legacy), on the target machine or container.
1. Configure the Agent to use eBPF for profiling. Refer to the [eBPF documentation](/docs/pyroscope/latest/configure-client/grafana-agent/ebpf) for detailed steps.
1. The collector collects eBPF profiles and sends them to the Pyroscope server.

### Supported languages

This eBPF profiler only collects CPU profiles. Generally natively compiled languages like C/C++, Go, Rust are supported (also see [Troubleshooting](https://grafana.com/docs/alloy/latest/reference/components/pyroscope/pyroscope.ebpf/#troubleshooting) for additional requirements).

The only high-level language supported is Python. Other high-level languages like Java, Ruby, PHP, JavaScript, etc. will require some additional work in order to correctly show stack traces of methods in these languages. Currently their CPU usage will be displayed belonging to the runtime's methods instead.

## Golang profiling in pull mode

In pull mode, the collector periodically retrieves profiles from Golang applications, specifically targeting the pprof endpoints.

### Benefits of Golang profiling in pull mode

- Non-intrusive: No need to modify your application’s source code.
- Centralized profiling: Suitable for environments with multiple Golang applications or microservices.
- Automatic: The agent handles the pulling and sending of profiles, requiring minimal configuration.

### Set up Golang profiling in pull mode

1. Ensure your Golang application exposes pprof endpoints.
1. Install and configure the collector, either Alloy or Agent, on the same machine or container where your application runs.
1. Ensure the collector is set to pull mode and targeting the correct pprof endpoints. For step-by-step instructions, visit the [Go (Pull Mode)](/docs/pyroscope/latest/configure-client/grafana-agent/go_pull) documentation.
1. The collector queries the pprof endpoints of your Golang application, collects the profiles, and forwards them to the Pyroscope server.

## Next steps

Whether using eBPF for versatile system and application profiling or relying on Golang's built-in pprof endpoints in pull mode, Grafana Agent and Grafana Alloy collectors offer streamlined processes to gather essential profiling data.
Choose the method that best fits your application and infrastructure needs.
