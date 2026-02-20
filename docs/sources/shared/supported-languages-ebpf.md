---
headless: true
description: Shared file for supported languages when using eBPF.
---

[//]: # 'This file documents the supported languages when using eBPF in Pyroscope.'
[//]: # 'This shared file is included in these locations:'
[//]: # '/pyroscope/docs/sources/configure-client/grafana-alloy/_index.md'
[//]: # '/pyroscope/docs/sources/configure-client/grafana-alloy/ebpf/_index.md'
[//]: #
[//]: # 'If you make changes to this file, verify that the meaning and content are not changed in any place where the file is included.'
[//]: # 'Any links should be fully qualified and not relative: /docs/grafana/ instead of ../grafana/.'

The eBPF profiler collects CPU profiles.
Natively compiled languages like C/C++, Go, Rust, and Zig are supported. Frame pointers are not required — the profiler uses `.eh_frame` data for unwinding.
Refer to [Troubleshooting unknown symbols](https://grafana.com/docs/pyroscope/latest/configure-client/grafana-alloy/ebpf/troubleshooting/#troubleshoot-unknown-symbols) for additional requirements and information.

The following high-level languages are also supported: Java (Hotspot JVM), .NET, Python, Ruby, PHP, Node.js, and Perl.
Each high-level language can be individually enabled or disabled in the [pyroscope.ebpf](https://grafana.com/docs/alloy/latest/reference/components/pyroscope/pyroscope.ebpf/) Alloy component configuration.
