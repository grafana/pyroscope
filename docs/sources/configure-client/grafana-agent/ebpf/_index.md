---
title: "Profiling with eBPF with Grafana Agent"
menuTitle: "Profiling with eBPF"
description: "Learn about using eBPF for continuous profiling for performance optimization."
weight: 20
aliases:
  - /docs/phlare/latest/configure-client/language-sdks/ebpf/
  - /docs/pyroscope/next/configure-client/language-sdks/ebpf
---

# Profiling with eBPF with Grafana Agent

<img src="/media/docs/pyroscope/ebpf_logo_color_on_white.png" width="100px;" alt="eBPF"/>

{{< youtube id="UX5aeL5KeZs" >}}

eBPF is an advanced technology embedded into the Linux kernel. It stands for enhanced [Berkeley Packet Filter](https://en.wikipedia.org/wiki/EBPF) and revolutionizes the capability to run sandboxed code safely within the kernel space. This technology serves multiple use cases, such as networking, security, and performance monitoring without the need to alter kernel code or load additional modules.

## Benefits and tradeoffs of using eBPF for continuous profiling

When it comes to application profiling, eBPF shines due to its high efficiency and minimal performance overhead.
eBPF enables the dynamic insertion of powerful monitoring code into live production systems.
By leveraging eBPF, developers can gain insights into application behavior, track resource usage, and detect bottlenecks in a way that traditional profiling tools cannot match.
eBPF's low overhead and fine-grained data collection make it an ideal choice for continuous, real-time profiling in performance-sensitive environments.

However, eBPF has some limitations that make it unsuitable for certain use cases:

- It isn't a good fit for profiling applications that are not written in a supported language.
- It can't be used to profile applications that are not running on Linux.
- It does not support all profile types such as memory and contention/lock profiling.
- eBPF requires root access to the host machine, which can be a problem in some environments.

## Supported languages

This eBPF profiler only collects CPU profiles. Generally, natively compiled languages like C/C++, Go, and Rust are supported. Refer to [Troubleshooting unknown symbols][troubleshooting] for additional requirements.

Python is the only supported high-level language, as long as `python_enabled=true`.
Other high-level languages like Java, Ruby, PHP, and JavaScript require additional work to show stack traces of methods in these languages correctly.
Currently, the CPU usage for these languages is reported as belonging to the runtime's methods.


## eBPF via the Grafana Agent

{{< docs/shared lookup="agent-deprecation.md" source="alloy" version="next" >}}

The Grafana Agent is a lightweight, all-in-one agent that can collect, transform, and ship observability data.
For profiling, the Grafana Agent can be configured to collect eBPF profiles and send them to Pyroscope.

This section contains instructions for installing and configuring the Grafana Agent to collect eBPF profiles.
For more information about the Grafana Agent itself, see the [Grafana Agent documentation](/docs/agent/latest/flow/).

[troubleshooting]: /docs/alloy/latest/reference/components/pyroscope/pyroscope.ebpf/#troubleshooting-unknown-symbols
