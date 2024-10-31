---
title: "Link tracing and profiling with Span Profiles"
menuTitle: "Link traces and profiles"
description: "Learn how to configure the client to Link tracing and profiling with span profiles."
weight: 400
---

# Link tracing and profiling with Span Profiles

Span Profiles are a powerful feature that further enhances the value of continuous profiling.
Span Profiles offer a novel approach to profiling by providing detailed insights into specific execution scopes of applications, moving beyond the traditional system-wide analysis to offer a more dynamic, focused analysis of individual requests or trace spans.

This method enhances understanding of application behavior by directly linking traces with profiling data, enabling engineering teams to pinpoint and resolve performance bottlenecks with precision.

Key benefits and features:

- Deep analysis: Understand the specifics of code execution within particular time frames, offering granular insights into application performance
- Seamless integration: Smoothly transition from a high-level trace overview to detailed profiling of specific trace spans within Grafana’s trace view
- Efficiency and cost savings: Quickly identify and address performance issues, reducing troubleshooting time and operational costs

{{< admonition type="note">}}
Span profiling is only effective on spans longer than 20ms to ensure statistical accuracy. 
{{< /admonition >}}

## Get started

Select an option from the list below:

- Configure Pyroscope: Begin sending profiling data to unlock the full potential of Span Profiles
- Client-side packages: Easily link traces and profiles using available packages for Go, Java, Ruby, .NET, and Python
  - Go: [Span profiles with Traces to profiles (Go)](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/trace-span-profiles/go-span-profiles/)
  - Java: [Span profiles with Traces to profiles (Java)](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/trace-span-profiles/java-span-profiles/)
  - Ruby: [Span profiles with Traces to profiles (Ruby)](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/trace-span-profiles/ruby-span-profiles/)
  - .NET: [Span profiles with Traces to profiles (.NET)](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/trace-span-profiles/dotnet-span-profiles/)
  - Python: [Span profiles with Traces to profiles (Python)](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/trace-span-profiles/python-span-profiles/)
- [Configure the Tempo data source in Grafana or Grafana Cloud](/docs/grafana-cloud/connect-externally-hosted/data-sources/tempo/configure-tempo-data-source/) to discover linked traces and profiles.

To learn more, check out the product announcement blog: [Introducing Span Profiles](/blog/2024/02/06/combining-tracing-and-profiling-for-enhanced-observability-introducing-span-profiles/).
