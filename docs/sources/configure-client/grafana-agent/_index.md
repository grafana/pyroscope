---
title: "Grafana Agent"
menuTitle: "Grafana Agent"
description: "Send data from your application via the Grafana Agent"
weight: 10
aliases:
  - /docs/phlare/latest/configure-client/grafana-agent/
---

# Grafana Agent

 
{{< docs/shared lookup="agent-deprecation.md" source="alloy" version="next" >}}

Grafana Agent is a powerful tool for collecting and forwarding profiling data.
With the introduction of support for eBPF and continuing support for Golang in pull mode, Grafana Agent has become even more versatile in its capabilities.
This document provides an overview of these two modes of profiling and guides users on setting them up.

{{< admonition type="note" >}}
Refer to [Available profiling types](../../view-and-analyze-profile-data/profiling-types/#available-profile-types) for a list of profile types supported.
{{< /admonition >}}


## eBPF profiling with Grafana Agent

eBPF (Extended Berkeley Packet Filter) is a modern Linux kernel technology that allows for safe, efficient, and customizable tracing of system and application behaviors without modifying the source code or restarting processes.

Benefits of eBPF profiling:

- Low overhead: eBPF collects data with minimal impact on performance.
- Versatile: eBPF can trace system calls, network packets, and even user-space application logic.
- Dynamic: No need to recompile or restart applications. eBPF allows for live tracing.

 ### Set Up eBPF profiling with Grafana Agent:

1. Ensure your system runs a Linux kernel version 4.9 or newer.
1. Install Grafana Agent on the target machine or container.
1. Configure the Agent to use eBPF for profiling. Refer to the [eBPF documentation](/docs/pyroscope/latest/configure-client/grafana-agent/ebpf) for detailed steps.
1. The Agent collects eBPF profiles and sends them to the Pyroscope server.

## Golang profiling in pull mode with Grafana Agent

In pull mode, Grafana Agent periodically retrieves profiles from Golang applications, specifically targeting the pprof endpoints.

### Benefits of Golang profiling in pull mode

- Non-intrusive: No need to modify your application’s source code.
- Centralized profiling: Suitable for environments with multiple Golang applications or microservices.
- Automatic: The agent handles the pulling and sending of profiles, requiring minimal configuration.

### Set Up Golang profiling in pull mode

1. Ensure your Golang application exposes pprof endpoints.
1. Install and configure the Grafana Agent on the same machine or container where your application runs.
1. Ensure the agent is set to pull mode and targeting the correct pprof endpoints. For step-by-step instructions, visit the [Go (Pull Mode)](/docs/pyroscope/latest/configure-client/grafana-agent/go_pull) docs.
1. The Agent queries the pprof endpoints of your Golang application, collects the profiles, and forwards them to the Pyroscope server.

## Next steps

Whether using eBPF for versatile system and application profiling or relying on Golang's built-in pprof endpoints in pull mode, Grafana Agent offers a streamlined process to gather essential profiling data.
Choose the method that best fits your application and infrastructure needs.
