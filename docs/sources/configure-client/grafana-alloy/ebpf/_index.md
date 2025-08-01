---
title: "Set up profiling with eBPF with Grafana Alloy"
menuTitle: "Set up profiling with eBPF"
description: "Learn about using eBPF for continuous profiling for performance optimization."
weight: 20
aliases:
  - /docs/phlare/latest/configure-client/language-sdks/ebpf/
  - /docs/pyroscope/next/configure-client/language-sdks/ebpf
---

# Set up profiling with eBPF with Grafana Alloy

eBPF is an advanced technology embedded into the Linux kernel. It stands for enhanced [Berkeley Packet Filter](https://en.wikipedia.org/wiki/EBPF) and revolutionizes the capability to run sandboxed code safely within the kernel space. This technology serves multiple use cases, such as networking, security, and performance monitoring without the need to alter kernel code or load additional modules.

<img src="/media/docs/pyroscope/ebpf_logo_color_on_white.png" width="100px;" alt="eBPF"/>

{{< youtube id="UX5aeL5KeZs" >}}

## Benefits and tradeoffs of using eBPF for continuous profiling

When it comes to application profiling, eBPF offers high efficiency and minimal performance overhead.
eBPF enables the dynamic insertion of powerful monitoring code into live production systems.
By leveraging eBPF, developers can gain insights into application behavior, track resource usage, and detect bottlenecks in a way that traditional profiling tools cannot match.
eBPF's low overhead and fine-grained data collection make it an ideal choice for continuous, real-time profiling in performance-sensitive environments.

However, eBPF has some limitations that make it unsuitable for certain use cases:

- It isn't a good fit for profiling applications that arn't written in a supported language.
- It can't be used to profile applications that aren't running on Linux.
- It doesn't support all profile types such as memory and contention/lock profiling.
- eBPF requires root access to the host machine, which can be a problem in some environments.

## Supported languages

[//]: # 'Shared content for supported languages with eBPF'
[//]: # 'This content is located in /pyroscope/docs/sources/shared/supported-languages-ebpf.md'

{{< docs/shared source="pyroscope" lookup="supported-languages-ebpf.md" version="latest" >}}

## eBPF using Alloy

Grafana Alloy is a lightweight, all-in-one collector that can collect, transform, and ship observability data.
For profiling, you can configure Alloy to collect eBPF profiles and send them to Pyroscope.

This section contains instructions for installing and configuring Alloy to collect eBPF profiles.
For more information about Alloy itself, refer to the [Alloy documentation](https://grafana.com/docs/alloy/<ALLOY_VERSION>/).

[troubleshooting]: /docs/alloy/<ALLOY_VERSION>/reference/components/pyroscope/pyroscope.ebpf/#troubleshooting-unknown-symbols
