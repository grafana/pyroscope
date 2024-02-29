---
title: "Linking tracing and profiling with span profiles"
menuTitle: "Linking traces and profiles"
description: "Learn how to configure the client to Link tracing and profiling with span profiles."
weight: 35
---

# Linking tracing and profiling with span profiles

Span Profiles are a powerful feature that further enhances the value of continuous profiling. Span Profiles offer a novel approach to profiling by providing detailed insights into specific execution scopes of applications, moving beyond the traditional system-wide analysis to offer a more dynamic, focused analysis of individual requests or trace spans. 

This method enhances understanding of application behavior by directly linking traces with profiling data, enabling engineering teams to pinpoint and resolve performance bottlenecks with precision.

Key Benefits and Features:

- Deep Analysis: Understand the specifics of code execution within particular time frames, offering granular insights into application performance
- Seamless Integration: Smoothly transition from a high-level trace overview to detailed profiling of specific trace spans within Grafana’s trace view
- Efficiency and Cost Savings: Quickly identify and address performance issues, reducing troubleshooting time and operational costs

Get Started:

- Configure Pyroscope: Begin sending profiling data to unlock the full potential of Span Profiles
- Client-Side Packages: Easily link traces and profiles using available packages for Go, Ruby, and Java
<<<<<<< HEAD
  - Go: [Span profiles with Traces to profiles (Go)]({{< relref "./go-span-profiles" >}})
  - Java: [Span profiles with Traces to profiles (Java)]({{< relref "./java-span-profiles" >}})
  - Ruby: [Span profiles with Traces to profiles (Ruby)]({{< relref "./ruby-span-profiles" >}})
=======
  - Go: [Span profiles with Traces to profiles (Go)]({{< relref "./docs/pyroscope/latest/configure-client/trace-span-profiles/go-span-profiles" >}})
  - Java: [Span profiles with Traces to profiles (Java)]({{< relref "./docs/pyroscope/latest/configure-client/trace-span-profiles/java-span-profiles" >}})
  - Ruby: [Span profiles with Traces to profiles (Ruby)]({{< relref "./docs/pyroscope/latest/configure-client/trace-span-profiles/ruby-span-profiles" >}})
>>>>>>> 2850220fe8819711aa1b0c4a970158cebcf1209c
- Grafana Tempo: Visualize and analyze Span Profiles within the Grafana Tempo UI

To learn more check out our product announcement blog introducing the [span profiles feature](/blog/2024/02/06/combining-tracing-and-profiling-for-enhanced-observability-introducing-span-profiles/)