---
title: "eBPF"
menuTitle: "eBPF"
description: "Understanding eBPF: Continuous Profiling for Performance Optimization"
weight: 20
aliases:
  - /docs/phlare/latest/configure-client/language-sdks/ebpf/
  - /docs/pyroscope/next/configure-client/language-sdks/ebpf
---


# Profiling with <img src="https://upload.wikimedia.org/wikipedia/commons/thumb/b/b0/EBPF_logo.png/240px-EBPF_logo.png" width="100px;" alt="eBPF"/>

eBPF is an advanced technology embedded into the Linux kernel. It stands for enhanced [Berkeley Packet Filter](https://en.wikipedia.org/wiki/EBPF) and revolutionizes the capability to run sandboxed code safely within the kernel space. This technology serves multiple use cases, such as networking, security, and performance monitoring without the need to alter kernel code or load additional modules.

## Benefits and Tradeoffs of using eBPF for continuous profiling

When it comes to application profiling, eBPF shines due to its high efficiency and minimal performance overhead. It enables the dynamic insertion of powerful monitoring code into live production systems. By leveraging eBPF, developers can gain insights into application behavior, track resource usage, and detect bottlenecks in a way that traditional profiling tools cannot match. eBPF's low overhead and fine-grained data collection make it an ideal choice for continuous, real-time profiling in performance-sensitive environments.

However, eBPF is not a silver bullet. It has some limitations that make it unsuitable for certain use cases:

- For example, eBPF is not a good fit for profiling applications that are not written in a supported language.
- It also cannot be used to profile applications that are not running on Linux.
- It does not support all profile types such as memory and contention/lock profiling.
- Finally, setting up eBPF requires root access to the host machine, which can be a problem in some environments.

We recommend an hybrid approach for better results: eBPF to profile the node and specific language instrumentation per application.

> We added a blog post "[The pros and cons of eBPF profiling](https://pyroscope.io/blog/ebpf-profiling-pros-cons)" which more deeply
explores this topic and provides some examples of eBPF profiles. If you're interested in some of the more granular details you can find them there!

## Grafana Agent

The Grafana Agent is a lightweight, all-in-one agent that can collect, transform, and ship observability data. For profiling, the Grafana Agent can be configured to collect eBPF profiles and send them to Pyroscope.

This section contains instructions for installing and configuring the Grafana Agent to collect eBPF profiles. For more information about the Grafana Agent itself, see the [Grafana Agent documentation](https://grafana.com/docs/agent/latest/flow/).
