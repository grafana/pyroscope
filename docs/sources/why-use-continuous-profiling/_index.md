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

## The Core Benefits of Continuous Profiling

![Diagram showing 3 benefits of continuous profiling](#)

Why prioritize continuous profiling? Here are the key reasons:
1. **In-Depth Code Insights:** It provides granular, line-level insights into how application code utilizes resources, offering the most detailed view of application performance.
2. **Complements Other Observability Tools:** Continuous profiling fills critical gaps left by metrics, logs, and tracing, creating a more comprehensive observability strategy.
3. **Proactive Performance Optimization:** Regular profiling enables teams to proactively identify and resolve performance bottlenecks, leading to more efficient and reliable applications.

## Business Impact of Continuous Profiling

![Infographic illustrating key business benefits](#)

Adopting continuous profiling with tools like Pyroscope can lead to significant business advantages:
1. **Reduced Operational Costs:** Optimization of resource usage can significantly cut down cloud and infrastructure expenses
2. **Latency reduction:** Identifying and addressing performance bottlenecks leads to faster and more efficient applications
3. **Enhanced Incident Management:** Faster problem identification and resolution, reducing Mean Time to Resolution (MTTR) and improving end-user experience

### How Pyroscope helps cut costs
Pyroscope's low-overhead profiling enables precise optimization of resource usage, directly impacting various cost centers in technology infrastructure. By providing in-depth insights into application performance, Pyroscope allows teams to identify and eliminate inefficiencies, leading to significant savings in areas like observability, incident management, messaging/queuing, deployment tools, and infrastructure.

### How Pyroscope helps reduce latency
Pyroscope plays a pivotal role in reducing application latency by identifying performance bottlenecks at the code level. This granular insight allows for targeted optimization, leading to faster application response times, improved user experience, and consequently, better business outcomes like increased customer satisfaction and revenue.

### How Pyroscope helps with incident management
Pyroscope streamlines incident management by offering immediate, actionable insights into application performance issues. With continuous profiling, teams can quickly pinpoint the root cause of an incident, reducing the mean time to resolution (MTTR) and enhancing overall system reliability and user satisfaction.

## Flamegraphs: Visualizing Performance Data

![Example of a flamegraph](#)

A fundamental aspect of continuous profiling is the flamegraph, an innovative way to visualize performance data. These graphs provide a clear, intuitive understanding of resource allocation and bottlenecks within the application. Pyroscope extends this functionality with additional visualization formats like tree graphs and top lists.

This diagram shows how code is turned into a flamegraph. In this case Pyroscope would sample the stacktrace of your application to understand how many CPU cycles are being spent in each function. It would then aggregate this data and turn it into a flamegraph. This is a very simplified example but it gives you an idea of how Pyroscope works.

Horizontally, the flamegraph represents 100% of the time that this application was running. The width of each node represents the amount of time spent in that function. The wider the node, the more time spent in that function. The narrower the node, the less time spent in that function.

Vertically, the nodes in the flamegraph represent the heirarchy of which functions were called and how much time was spent in each function. The top node is the root node and represents the total amount of time spent in the application. The nodes below it represent the functions that were called and how much time was spent in each function. The nodes below those represent the functions that were called from those functions and how much time was spent in each function. This continues until you reach the bottom of the flamegraph.

This is a cpu profile, but profiles can represent many other types of resource such as memory, network, disk, etc. To understand more about how to read a flamegraph, what the different colors mean, and what other types of profiles exist and when to use them see our [flamegraph 101 documention] or [when to use which profile type documentation].

## Pyroscope: Continuous Profiling in Action

![Screenshots of Pyroscope's UI](#)

Pyroscope's UI is designed to make it easy to visualize and analyze profiling data. There are several different modes for viewing, analyzing, uploading, and comparing profiling data. We will go into more detail about these modes in the [Pyroscope UI documentation]. For now, it is important to note that one of the major benefits of continuous profiling is the ability to compare and diff profiling data from two different queries:
- Comparing two different git commits before and after a code change
- Comparing Staging vs production environments to identify differences in performance
- Comparing performance between two different a/b tests or feature flag experiments
- Comparing memory allocations between two different time periods before and after a memory leak
- etc 

With traditional profiling getting any of this information is much more difficult to organize, properly label, share, or store for later analysis. With Pyroscope, all of this is just a matter of writing the two queries you'd like to compare and clicking a button.


## Seamless Integration with Observability Tools

![Flowchart showing Pyroscope integration with other tools](#)

Pyroscope enhances its value through seamless integration with leading observability tools like Grafana, Prometheus, and Loki. This integration facilitates deeper insights into application performance and aids in addressing issues identified through other monitoring methods.

## Getting Started with Pyroscope

![Guide for instrumenting an application with Pyroscope](#)

Begin your journey with Pyroscope. Visit our [Getting Started Guide](link-to-getting-started) to learn about the different ways to instrument your application with Pyroscope. Join our [community](link-to-community) and contribute to the evolving world of continuous profiling.
