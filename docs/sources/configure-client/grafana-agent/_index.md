---
title: "Grafana Agent"
menuTitle: "Grafana Agent"
description: "Send data from your application via the Grafana Agent"
weight: 10
aliases:
  - /docs/phlare/latest/configure-client/grafana-agent/
---

# Grafana Agent

The Grafana Agent is a powerful tool for collecting and forwarding profiling data. Recently, with the introduction of support for eBPF and continuing support for Golang in pull mode, the Grafana Agent has become even more versatile in its capabilities. This document provides an overview of these two modes of profiling and guides users on setting them up.

## eBPF Profiling with Grafana Agent

eBPF (Extended Berkeley Packet Filter) is a modern Linux kernel technology that allows for safe, efficient, and customizable tracing of system and application behaviors without modifying the source code or restarting processes.

**Benefits of eBPF Profiling:**

- **Low Overhead**: eBPF collects data with minimal impact on performance.
- **Versatile**: eBPF can trace system calls, network packets, and even user-space application logic.
- **Dynamic**: No need to recompile or restart applications. eBPF allows for live tracing.

**How to Set Up eBPF Profiling with the Grafana Agent:**

1. Ensure your system runs a Linux kernel version 4.9 or newer.
2. Install the Grafana Agent on the target machine or container.
3. Configure the agent to use eBPF for profiling. Refer to the [eBPF documentation](/docs/pyroscope/latest/configure-client/grafana-agent/ebpf) for detailed steps.
4. Once configured, the agent will start collecting eBPF profiles and send them to the Pyroscope server.

## Golang Profiling in Pull Mode with Grafana Agent

In pull mode, the Grafana Agent periodically retrieves profiles from Golang applications, specifically targeting the pprof endpoints.

**Benefits of Golang Profiling in Pull Mode:**

- **Non-Intrusive**: No need to modify your application’s source code.
- **Centralized Profiling**: Suitable for environments with multiple Golang applications or microservices.
- **Automatic**: The agent handles the pulling and sending of profiles, requiring minimal configuration.

**How to Set Up Golang Profiling in Pull Mode:**

1. Ensure your Golang application exposes pprof endpoints.
2. Install and configure the Grafana Agent on the same machine or container where your application runs.
3. Ensure the agent is set to pull mode and targeting the correct pprof endpoints. For step-by-step instructions, visit the [Go (Pull Mode)](/docs/pyroscope/latest/configure-client/grafana-agent/go_pull) docs.
4. The agent will start querying the pprof endpoints of your Golang application, collecting the profiles, and forwarding them to the Pyroscope server.

## Next steps

Whether using eBPF for versatile system and application profiling or relying on Golang's built-in pprof endpoints in pull mode, the Grafana Agent offers a streamlined process to gather essential profiling data. Choose the method that best fits your application and infrastructure needs.

For additional details, examples, or troubleshooting, please visit the links provided above. And as always, the Pyroscope team is available on Slack or GitHub for further assistance.
