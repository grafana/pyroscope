---
title: Profile types and instrumentation
menuTitle: Profile types and instrumentation
description: Learn about the different profiling types available in Pyroscope and
weight: 100
aliases:
  - ../ingest-and-analyze-profile-data/profiling-types/
  - ../view-and-analyze-profile-data/profiling-types/ # /docs/pyroscope/latest/view-and-analyze-profile-data/profiling-types/
keywords:
  - pyroscope
  - profiling types
  - application performance
  - flame graphs
---

# Profile types and instrumentation

Profiling is an essential tool for understanding and optimizing application performance. In Pyroscope, various profiling types allow for an in-depth analysis of different aspects of your application. This guide explores these types and explain their impact on your program.

Profiling types refer to different dimensions of application performance analysis, focusing on specific aspects like CPU usage, memory allocation, or thread synchronization.

[//]: # 'Shared content for available profile types'
[//]: # 'This content is located in /pyroscope/docs/sources/shared/available-profile-types.md'

{{< docs/shared source="pyroscope" lookup="available-profile-types.md" version="latest" >}}

Refer to [Understand profiling types and their uses in Pyroscope](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/introduction/profiling-types/) for more details about the profile types.

## Profile type support by instrumentation method

The instrumentation method you use determines which profile types are available. You can use either auto or manual instrumentation.

### Auto-instrumentation with Grafana Alloy

You can send data from your application using Grafana Alloy collector. Alloy supports profiling with eBPF, Java, and Golang in pull mode.

[//]: # 'Shared content for supported languages with eBPF'
[//]: # 'This content is located in /pyroscope/docs/sources/shared/supported-languages-ebpf.md'

{{< docs/shared source="pyroscope" lookup="supported-languages-ebpf.md" version="latest" >}}

For more information, refer to [Configure the client to send profiles with Grafana Alloy](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/grafana-alloy/).

This table lists the available profile types based on auto instrumentation using Alloy.

| Profile type   | Go (pull) | Java | eBPF      |
| -------------- | --------- | ---- | --------- |
| CPU            | Yes       | Yes  | Yes       |
| Alloc Objects  | Yes       | Yes  |           |
| Alloc Space    | Yes       | Yes  |           |
| Inuse Objects  |           |      |           |
| Inuse Space    |           |      |           |
| Goroutines     | Yes       |      |           |
| Mutex Count    |           |      |           |
| Mutex Duration |           |      |           |
| Block Count    | Yes       |      |           |
| Block Duration | Yes       |      |           |
| Lock Count     |           | Yes  |           |
| Lock Duration  |           | Yes  |           |
| Exceptions     |           |      |           |
| Wall           |           |      |           |
| Heap           |           |      |           |

### Instrumentation with SDKs

Using the Pyroscope language SDKs lets you instrument your application directly for precise profiling. You can customize the profiling process according to your application’s specific requirements.

For more information on the language SDKs, refer to [Pyroscope language SDKs](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/language-sdks/).

This table lists the available profile types based on the language SDK.

| Profile type   | Go (push) | Java | .NET       | Ruby | Python | Rust | Node.js |
| -------------- | --------- | ---- | ---------- | ---- | ------ | ---- | ------- |
| CPU            | Yes       | Yes  | Yes        | Yes  | Yes    | Yes  | Yes     |
| Alloc Objects  | Yes       | Yes  | Yes        |      |        |      |         |
| Alloc Space    | Yes       | Yes  | Yes        |      |        |      |         |
| Inuse Objects  | Yes       |      | Yes (7.0+) |      |        |      |         |
| Inuse Space    | Yes       |      | Yes (7.0+) |      |        |      |         |
| Goroutines     | Yes       |      |            |      |        |      |         |
| Mutex Count    | Yes       |      | Yes        |      |        |      |         |
| Mutex Duration | Yes       |      | Yes        |      |        |      |         |
| Block Count    | Yes       |      |            |      |        |      |         |
| Block Duration | Yes       |      |            |      |        |      |         |
| Lock Count     |           | Yes  | Yes        |      |        |      |         |
| Lock Duration  |           | Yes  | Yes        |      |        |      |         |
| Exceptions     |           |      | Yes        |      |        |      |         |
| Wall           |           |      | Yes        |      |        |      | Yes     |
| Heap           |           |      | Yes (7.0+) |      |        |      | Yes     |

## Profile types supported with span profiles

Pyroscope can integrate with distributed tracing systems supporting the OpenTelemetry standard. This integration lets you link traces with the profiling data and find resource usage for specific lines of code for your trace spans.

Only CPU profile type is supported for span profiles.

The following languages are supported:

- [Go](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/trace-span-profiles/go-span-profiles/)
- [Java](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/trace-span-profiles/java-span-profiles/)
- [Ruby](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/trace-span-profiles/ruby-span-profiles/)
- [.NET](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/trace-span-profiles/dotnet-span-profiles/)
- [Python](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/trace-span-profiles/python-span-profiles/)
