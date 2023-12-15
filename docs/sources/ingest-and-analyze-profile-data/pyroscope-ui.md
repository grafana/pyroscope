---
title: Use Pyroscope UI
menuTitle: Use the Pyroscope UI
description: How to use the Pyroscope UI to analyze performance of your applications.
weight: 40
keywords:
  - pyroscope
  - performance analysis
  - flamegraphs
---

## Pyroscope: Continuous Profiling in Action

![Screenshots of Pyroscope's UI](https://grafana.com/static/img/pyroscope/pyroscope-ui-diff-2023-11-30.png)

Pyroscope's UI is designed to make it easy to visualize and analyze profiling data.
There are several different modes for viewing, analyzing, uploading, and comparing profiling data.
We will go into more detail about these modes in the [Pyroscope UI documentation].
For now, it's important to note that one of the major benefits of continuous profiling is the ability to compare and diff profiling data from two different queries:

- Comparing two different git commits before and after a code change
- Comparing Staging vs production environments to identify differences in performance
- Comparing performance between two different a/b tests or feature flag experiments
- Comparing memory allocations between two different time periods before and after a memory leak
- etc

With traditional profiling getting any of this information is much more difficult to organize, properly label, share, or store for later analysis. With Pyroscope, all of this is just a matter of writing the two queries you'd like to compare and clicking a button.

This UI will also expand over time to better help dig deeper into the data and provide more insights into your application.

## Seamless integration with observability tools

![Flowchart showing Pyroscope integration with other tools](https://grafana.com/static/img/pyroscope/grafana-pyroscope-dashboard-2023-11-30.png)

Pyroscope enhances its value through seamless integration with leading observability tools like Grafana, Prometheus, and Loki. This integration facilitates deeper insights into application performance and aids in addressing issues identified through other monitoring methods.