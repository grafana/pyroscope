---
title: When to use continuous profiling
menuTitle: When to use continuous profiling
description: Discover the benefits of continuous profiling and its role in modern application performance analysis.
weight: 20
keywords:
  - pyroscope
  - phlare
  - continuous profiling
  - flamegraphs
---

## When to use continuous profiling

**Continuous profiling** is a systematic method of collecting and analyzing performance data from production systems.

Traditionally, profiling has been used more as an ad-hoc debugging tool in languages like Go and Java.
You are probably used to running a benchmark tool locally and getting a pprof file in Go or maybe connecting into a misbehaving prod instance and pulling a flamegraph from a JFR file in Java.
This is great for debugging but not so great for production.

![example flamegraph](https://grafana.com/static/img/pyroscope/pyroscope-ui-single-2023-11-30.png)

Continuous profiling is a  modern approach which is safer and more scalable for production environments.
It makes use of low overhead sampling to collect profiles from production systems and stores them in a database for later analysis.
You can get a more holistic view of your application and how it behaves in production.

## Benefits

![Diagram showing 3 benefits of continuous profiling](https://grafana.com/static/img/pyroscope/profiling-use-cases-diagram.png)

Why prioritize continuous profiling?

1. **In-Depth Code Insights:** It provides granular, line-level insights into how application code utilizes resources, offering the most detailed view of application performance.
2. **Complements Other Observability Tools:** Continuous profiling fills critical gaps left by metrics, logs, and tracing, creating a more comprehensive observability strategy.
3. **Proactive Performance Optimization:** Regular profiling enables teams to proactively identify and resolve performance bottlenecks, leading to more efficient and reliable applications.

## Use cases

![Infographic illustrating key business benefits](https://grafana.com/static/img/pyroscope/cost-cutting-diagram.png)

Adopting continuous profiling with tools like Pyroscope can lead to significant business advantages:

1. **Reduced operational costs:** Optimization of resource usage can significantly cut down cloud and infrastructure expenses
2. **Reduced latency:** Identifying and addressing performance bottlenecks leads to faster and more efficient applications
3. **Enhanced incident management:** Faster problem identification and resolution, reducing Mean Time to Resolution (MTTR) and improving end-user experience

### Reduce operational costs

Pyroscope's low-overhead profiling enables precise optimization of resource usage, directly impacting various cost centers in technology infrastructure.
By providing in-depth insights into application performance, Pyroscope allows teams to identify and eliminate inefficiencies, leading to significant savings in areas like observability, incident management, messaging/queuing, deployment tools, and infrastructure.

### Reduced latency

Pyroscope plays a pivotal role in reducing application latency by identifying performance bottlenecks at the code level.
This granular insight allows for targeted optimization, leading to faster application response times, improved user experience, and consequently, better business outcomes like increased customer satisfaction and revenue.

### Enhanced incident management

Pyroscope streamlines incident management by offering immediate, actionable insights into application performance issues.
With continuous profiling, teams can quickly pinpoint the root cause of an incident, reducing the mean time to resolution (MTTR) and enhancing overall system reliability and user satisfaction.