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

The eBPF profiler only collects CPU profiles.
Generally, natively compiled languages like C/C++, Go, and Rust are supported. They should have frame pointers enabled (enabled by default in Go).
Refer to [Troubleshooting unknown symbols][https://grafana.com/docs/pyroscope/latest/configure-client/grafana-alloy/ebpf/troubleshooting/#troubleshoot-unknown-symbols] for additional requirements and information.

Python is the only supported high-level language, as long as `python_enabled=true`.
Other high-level languages like Java, Ruby, PHP, and JavaScript require additional work to show stack traces of methods in these languages correctly.
Currently, the CPU usage for these languages is reported as belonging to the runtime's methods.
