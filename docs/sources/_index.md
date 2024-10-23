---
title: "Grafana Pyroscope"
weight: 1
description: Grafana Pyroscope is an open source software project for aggregating continuous profiling data.
keywords:
  - Grafana Pyroscope
  - Grafana profiles
  - TSDB
  - profiles storage
  - profiles datastore
  - observability
  - continuous profiling
hero:
  title: Grafana Pyroscope
  level: 1
  image: /static/img/pyroscope-logo.svg
  width: 110
  height: 110
  description: >-
    Grafana Pyroscope is an open source software project for aggregating continuous profiling data. Continuous profiling is an observability signal that allows you to understand your workload's resources usage down to the source code line number.
    Grafana Pyroscope is fully integrated with Grafana allowing you to correlate with other observability signals, like metrics, logs, and traces.
cards:
  title_class: pt-0 lh-1
  items:
    - title: Learn about profiling
      href: /docs/pyroscope/latest/introduction/
      description: Learn about continuous profiling and how you identify performance bottlenecks and optimize your applications. After an application is profiled, you can start with system-wide observability and drill down to actionable code-level insights.
    - title: Get started with Pyroscope
      href: ./get-started/
      description: Learn how to install and configure Grafana Pyroscope with several examples.
    - title: Instrument your app and configure the client
      href: ./configure-client/
      description: When sending profiles to Pyroscope, you can choose between SDK instrumentation and auto-instrumentation using Grafana Alloy. This document explains these two techniques and helps you choose one.
    - title: Configure the server
      href: ./configure-server/
      description: Configure your Pyroscope server to meet your needs by setting disk storage, tenant IDs, memberlist, proxies, shuffle sharding, and more. You can also use the server HTTP API.
    - title: View and analyze profile data
      href: ./view-and-analyze-profile-data/
      description: Profiling data can be presented in a variety of formats presents, including flame graphs, tables, as well as charts and graphs. Flame graphs visualize call relationships and identify hot spots. Tables let you view detailed statistics for specific functions or time periods. Charts and graphs help you analyze trends and compare performance across different metrics.
    - title: Pyroscope architecture
      href: ./reference-pyroscope-architecture/
      description: Take a deep-dive into the microservices-based architecture to learn about deployment modes, components (microservices), and more. The system has multiple horizontally scalable microservices that can run separately and in parallel.
---

{{< docs/hero-simple key="hero" >}}

---

## Overview

Grafana Pyroscope is a multi-tenant, continuous profiling aggregation system, aligning its architectural design with Grafana Mimir, Grafana Loki, and Grafana Tempo.
This integration enables a cohesive correlation of profiling data with existing metrics, logs, and traces.

Explore continuous profiling data to gain insights into application performance.
You can query and analyze production data in a structured way.
Use the Pyroscope UI or Grafana to visualize the data.

<!--video style="border-radius: 1%; width: 75%; display: block; margin-left: auto; margin-right: auto;" autoplay loop>
  <source src="ui.webm" type="video/webm">
</video-->
![Pyroscope UI showing the comparison view](/media/docs/pyroscope/screenshot-pyroscope-comp-view.png)

## Explore

{{< card-grid key="cards" type="simple" >}}
