---
title: Introduction
menuTitle: Introduction
description: Learn about Pyrsocope and profiling.
weight: 10
keywords:
  - Pyroscope
  - Profiling
---

# Introduction

Grafana Pyroscope is a multi-tenant continuous profiling aggregation system, aligning its architectural design with Grafana Mimir, Grafana Loki, and Grafana Tempo. It facilitates the ingestion, storage, and querying of profiles and seamlessly integrates with Grafana, enabling a cohesive correlation of profiling data with existing metrics, logs, and traces.

Engineers using Pyroscope gain the ability to delve deep into the performance attributes and resource demands of their applications.
Boasting an intuitive interface coupled with a vivid data visualization, Pyroscope transforms raw profiling data into readily actionable insights.

{{< youtube id="XL2yTCPy2e0" >}}

## Why Pyroscope

Continuous profiling helps teams to quickly identify performance bottlenecks and optimize their applications.
With Grafana Pyroscope, teams can easily profile their applications in production with minimal overhead.
Starting with system-wide observability and drilling down to actionable code-level insights allows teams to identify performance issues in context no matter where they occur, so they can optimize their applications with precision.

## Core functionality

With Pyroscope, you get access to the core profiling functionality, which you can use to find performance bottlenecks and optimize applications. The core functionality includes:

- Minimal CPU overhead and efficient compression ensure optimal performance.
- Architecture consistent with Loki, Mimir, and Tempo, promoting a smoother user experience.
    - Horizontally scalable
    - Reliable: Highly available setup ensures consistent uptime, even amidst upgrades or system failures.
    - Multi-tenancy Support: makes it possible to run one database for multiple independent teams or business units.
    - Cost Effective at Scale: Utilizes object storage, which allows extensive historical data storage without significant costs.
- Advanced Analysis UI: Provides an advanced UI, high-cardinality tag/label handling, and the ability to differentiate performance between tags/labels and time intervals.

