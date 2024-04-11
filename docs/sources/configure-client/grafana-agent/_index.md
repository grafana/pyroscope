---
title: "Grafana Agent"
menuTitle: "Grafana Agent"
description: "Send data from your application via the Grafana Agent"
weight: 10
aliases:
  - /docs/phlare/latest/configure-client/grafana-agent/
---

# Grafana Agent

Grafana Agent is a powerful tool for collecting and forwarding profiling data.
With the introduction of support for eBPF and continuing support for Golang in pull mode, Grafana Agent has become even more versatile in its capabilities.
This document provides an overview of these two modes of profiling and guides users on setting them up.

{{< docs/shared lookup="agent-deprecation.md" source="alloy" version="next" >}}

## eBPF profiling with Grafana Agent

eBPF (Extended Berkeley Packet Filter) is a modern Linux kernel technology that allows for safe, efficient, and customizable tracing of system and application behaviors without modifying the source code or restarting processes.

Benefits of eBPF profiling:

- Low overhead: eBPF collects data with minimal impact on performance.
- Versatile: eBPF can trace system calls, network packets, and even user-space application logic.
- Dynamic: No need to recompile or restart applications. eBPF allows for live tracing.

### Set up eBPF profiling with Grafana Agent

1. Ensure your system runs a Linux kernel version 4.9 or newer.
1. Install Grafana Agent or Grafana Alloy on the target machine or container.
1. Configure the agent to use eBPF for profiling. Refer to the [eBPF documentation](/docs/pyroscope/latest/configure-client/grafana-agent/ebpf) for detailed steps.
1. After it's configured, the agent starts collecting eBPF profiles and sends them to the Pyroscope server.

## Golang profiling in pull mode with Grafana Agent

In pull mode, Grafana Agent periodically retrieves profiles from Golang applications, specifically targeting the pprof endpoints.

### Benefits of Golang profiling in pull mode

- Non-intrusive: No need to modify your application’s source code.
- Centralized profiling: Suitable for environments with multiple Golang applications or microservices.
- Automatic: The agent handles the pulling and sending of profiles, requiring minimal configuration.

### Set up Golang profiling in pull mode

1. Ensure your Golang application exposes pprof endpoints.
2. Install and configure Grafana Agent on the same machine or container where your application runs.
3. Ensure the agent is set to pull mode and targeting the correct pprof endpoints. For step-by-step instructions, visit the [Go (Pull Mode)](/docs/pyroscope/latest/configure-client/grafana-agent/go_pull) docs.
4. The agent starta querying the pprof endpoints of your Golang application, collecting the profiles, and forwarding them to the Pyroscope server.

## Next steps

Whether using eBPF for versatile system and application profiling or relying on Golang's built-in pprof endpoints in pull mode, Grafana Agent and Grafana Alloy offer a streamlined process to gather essential profiling data. Choose the method that best fits your application and infrastructure needs.

