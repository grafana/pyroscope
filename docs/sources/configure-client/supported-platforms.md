---
title: Supported operating systems and architectures
menuTitle: Supported platforms
description: Operating system, CPU architecture, and libc support for each Pyroscope language SDK.
weight: 120
keywords:
  - pyroscope
  - language sdks
  - supported platforms
  - operating systems
  - architecture
---

# Supported operating systems and architectures

The Pyroscope [language SDKs](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/language-sdks/) instrument your application in-process to send profiles to Pyroscope. Because most SDKs rely on a native profiling engine, the operating systems and CPU architectures they support vary. This page summarizes what each SDK supports.

For the list of profile types each SDK produces, refer to [Profile types and instrumentation](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/profile-types/).

## Support matrix

| SDK | Linux x86_64 | Linux ARM64 | macOS x86_64 | macOS ARM64 | Windows x64 |
| --- | --- | --- | --- | --- | --- |
| Go | Yes | Yes | Yes | Yes | Yes |
| Java | Yes | Yes | Yes | Yes | No |
| .NET | Yes | Yes | No | No | Public preview (see note) |
| Python | Yes | Yes | Yes | Yes | No |
| Ruby | Yes | Yes | Yes | Yes | No |
| Rust | Yes | Yes | Yes | Yes | No |
| Node.js | Yes | Yes | Yes | Yes | Yes |

{{< admonition type="note" >}}
Windows support for the .NET SDK is in [public preview](https://grafana.com/docs/release-life-cycle/), starting with profiler version 1.3.0. On .NET 8 and later it has full parity with Linux; on .NET Framework 4.8 only CPU profiling is available. Refer to the [.NET SDK documentation](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/language-sdks/dotnet/).
{{< /admonition >}}


## Linux libc: glibc and musl (Alpine)

Most containerized deployments run on either `glibc` (for example, Debian or Ubuntu) or `musl` (Alpine) Linux. All SDKs support both, but they package it differently:

* **Go** and **Java** work on both from a single build. Go is pure Go, and the Java agent bundles one native library per architecture that runs on either `libc`.
* **.NET**, **Python**, and **Node.js** publish separate `glibc` and `musl` builds. For .NET, download the tarball that matches your base image; for Python (`pip`) and Node.js (`npm`), the matching build is selected automatically.
* **Ruby** ships precompiled gems for `glibc` Linux only. On Alpine, the `gem` compiles its native extension at install time, which requires a Rust toolchain.

## macOS

macOS is intended for local development, not production. The Go, Java, Python, Ruby, and Rust SDKs run on macOS for both Intel and Apple Silicon. The .NET SDK does not support macOS.

## Windows

The Go and Node.js SDKs run on Windows. The .NET SDK supports Windows x64 in [public preview](https://grafana.com/docs/release-life-cycle/) (see the note under the support matrix). The Java, Python, Ruby, and Rust SDKs don't support Windows.

## Auto-instrumentation with Grafana Alloy and eBPF

The support above applies to the language SDKs, which instrument your application in-process. [Auto-instrumentation with Grafana Alloy](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/grafana-alloy/), including the eBPF profiler, runs only on Linux because it depends on Linux kernel features. It isn't available on macOS or Windows.
