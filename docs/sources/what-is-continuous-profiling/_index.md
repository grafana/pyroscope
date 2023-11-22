---
title: "What is continuous profiling?"
menuTitle: "What is continuous profiling?"
description: "Learn about continuous profiling and how it fits into the broader observability context."
weight: 20
keywords:
  - pyroscope
  - phlare
  - continuous profiling
  - flamegraphs
---

# What is Continuous Profiling?

![Visual comparison between traditional and continuous profiling](#)

Continuous profiling is a modern approach to performance analysis, extending beyond traditional profiling methods. Traditional profiling, akin to "console.log debugging," provides transient, local insights useful for development but not scalable for production. In contrast, continuous profiling offers a formalized, centralized way to collect, store, and utilize production data profiles in a database. Pyroscope facilitates both continuous and traditional profiling, serving as a versatile tool for in-depth application analysis.

### Benefits of Continuous Profiling

![Diagram showing 3 benefits of continuous profiling](#)

Continuous profiling stands as the most fundamental method to gain granular insights into application code, providing line-level details on resource usage. It can be utilized both in isolation for a broad application overview or in conjunction with metrics, logs, and tracing for comprehensive observability. It acts as a critical link, bridging the gap between metrics, logs, and tracing.

### Business Advantages

![Infographic illustrating key business benefits](#)

Continuous profiling, particularly with Pyroscope, unlocks significant business benefits:
1. **Cost Cutting:** Efficiently reduces cloud spending.
2. **Application Latency Reduction:** Enhances application speed.
3. **Faster Incident Resolution:** Lowers Mean Time to Resolution (MTTR), streamlining troubleshooting processes.

### Understanding Flamegraphs

![Example of a flamegraph](#)

A core component of Pyroscope's functionality is the flamegraph, a visualization method for profiling data. It offers an intuitive representation of resource usage, making it easier to identify bottlenecks. Pyroscope extends beyond flamegraphs, supporting additional visualizations like tree graphs and top lists for diverse analytical perspectives.

### Pyroscope in Action: A Demo

![Screenshots of Pyroscope's UI](#)

Experience Pyroscope's capabilities through a live demonstration. While this section doesn't delve into instrumentation details, it highlights the user interface and the various modes available for analyzing profiling data.

### Integrating with Other Observability Tools

![Flowchart showing Pyroscope integration with other tools](#)

Discover how Pyroscope seamlessly integrates with popular observability tools like Grafana, Prometheus, and Loki. This synergy allows for the most detailed insights into application performance, assisting in identifying and resolving issues detected through metrics, logs, or traces.

### Get Started with Pyroscope

![Guide for instrumenting an application with Pyroscope](#)

Embark on your journey with Pyroscope. Follow our [Getting Started Guide](link-to-getting-started) and explore the various ways to instrument your application with Pyroscope. Your feedback and contributions are always welcome in our [community](link-to-community).
