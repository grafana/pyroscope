---
title: "Why Use Continuous Profiling?"
menuTitle: "Why Use Continuous Profiling?"
description: "Discover the benefits of continuous profiling and its role in modern application performance analysis."
weight: 20
keywords:
  - pyroscope
  - phlare
  - continuous profiling
  - flamegraphs
---

# Why Use Continuous Profiling?

![Visual comparison between traditional and continuous profiling](#)

Continuous profiling is more than just a performance analysis tool; it's a crucial component in modern software development and operations. It goes past traditional profiling techniques by providing ongoing, in-depth insights into application performance. 

Continuous profiling goes past the ephemeral, localized nature of traditional profiling (which historically has been more similar to "console.log" or "print statement" debugging) to a structured, centralized approach allows for effective use in production environments. Put more simply, Pyroscope takes you from a bunch of flamegraph files on your desktop to a database where you can query and analyze production data in a structured way.

Pyroscope in particular, offers you the flexibility to either visualize more "traditional" adhoc data or evolve your applications observability tooling to include more "modern" continuous profiling capabilities.

## What is Continuous Profiling?

Continuous profiling is a systematic method of collecting and analyzing performance data from production systems.

Traditionally, profiling has been used more as an ad-hoc debugging tool. While used in many languages, particularly in Go and Java many are used to running a benchmark tool locally and getting a pprof file in go or maybe ssh'ing into a misbehaving prod instance and pulling a flamegraph from a JFR file in Java. This is great for debugging but not so great for production.

Continuous profiling is a much more modern approach which is safer and more scalable for production environments. It makes use of low overhead sampling to collect profiles from production systems and stores them in a database for later analysis. This allows you to get a much more holistic view of your application and how it behaves in production.

### The Core Benefits of Continuous Profiling

![Diagram showing 3 benefits of continuous profiling](#)

Why prioritize continuous profiling? Here are the key reasons:
1. **In-Depth Code Insights:** It provides granular, line-level insights into how application code utilizes resources, offering the most detailed view of application performance.
2. **Complements Other Observability Tools:** Continuous profiling fills critical gaps left by metrics, logs, and tracing, creating a more comprehensive observability strategy.
3. **Proactive Performance Optimization:** Regular profiling enables teams to preemptively identify and resolve performance bottlenecks, leading to more efficient and reliable applications.

### Business Impact of Continuous Profiling

![Infographic illustrating key business benefits](#)

Adopting continuous profiling with tools like Pyroscope can lead to significant business advantages:
1. **Reduced Operational Costs:** Optimization of resource usage can significantly cut down cloud and infrastructure expenses.
2. **Improved Application Performance:** Identifying and addressing performance bottlenecks leads to faster and more efficient applications.
3. **Enhanced Incident Management:** Faster problem identification and resolution, reducing Mean Time to Resolution (MTTR).

### Flamegraphs: Visualizing Performance Data

![Example of a flamegraph](#)

A fundamental aspect of continuous profiling is the flamegraph, an innovative way to visualize performance data. These graphs provide a clear, intuitive understanding of resource allocation and bottlenecks within the application. Pyroscope extends this functionality with additional visualization formats like tree graphs and top lists.

### Pyroscope: Continuous Profiling in Action

![Screenshots of Pyroscope's UI](#)

Explore Pyroscope's capabilities through a hands-on demonstration. This section highlights the powerful user interface of Pyroscope and the various modes available for comprehensive profiling data analysis.

### Seamless Integration with Observability Tools

![Flowchart showing Pyroscope integration with other tools](#)

Pyroscope enhances its value through seamless integration with leading observability tools like Grafana, Prometheus, and Loki. This integration facilitates deeper insights into application performance and aids in addressing issues identified through other monitoring methods.

### Getting Started with Pyroscope

![Guide for instrumenting an application with Pyroscope](#)

Begin your journey with Pyroscope. Visit our [Getting Started Guide](link-to-getting-started) to learn about the different ways to instrument your application with Pyroscope. Join our [community](link-to-community) and contribute to the evolving world of continuous profiling.
