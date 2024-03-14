---
title: "Linking tracing and profiling with Span Profiles"
menuTitle: "Linking traces and profiles"
description: "Learn how to configure the client to Link tracing and profiling with span profiles."
weight: 35
---

# Linking tracing and profiling with Span Profiles

Span Profiles are a powerful feature that further enhances the value of continuous profiling.
Span Profiles offer a novel approach to profiling by providing detailed insights into specific execution scopes of applications, moving beyond the traditional system-wide analysis to offer a more dynamic, focused analysis of individual requests or trace spans.

This method enhances understanding of application behavior by directly linking traces with profiling data, enabling engineering teams to pinpoint and resolve performance bottlenecks with precision.

Key benefits and features:

- Deep analysis: Understand the specifics of code execution within particular time frames, offering granular insights into application performance
- Seamless integration: Smoothly transition from a high-level trace overview to detailed profiling of specific trace spans within Grafana’s trace view
- Efficiency and cost savings: Quickly identify and address performance issues, reducing troubleshooting time and operational costs

Get started:

- Configure Pyroscope: Begin sending profiling data to unlock the full potential of Span Profiles
- Client-Side Packages: Easily link traces and profiles using available packages for Go, Ruby, and Java
  - Go: [Span profiles with Traces to profiles (Go)]({{< relref "./go-span-profiles" >}})
  - Java: [Span profiles with Traces to profiles (Java)]({{< relref "./java-span-profiles" >}})
  - Ruby: [Span profiles with Traces to profiles (Ruby)]({{< relref "./ruby-span-profiles" >}})
- Grafana Tempo: Visualize and analyze Span Profiles within the Grafana using a Tempo data source.

To learn more, check out our product announcement blog: [Introducing Span Profiles](/blog/2024/02/06/combining-tracing-and-profiling-for-enhanced-observability-introducing-span-profiles/).
