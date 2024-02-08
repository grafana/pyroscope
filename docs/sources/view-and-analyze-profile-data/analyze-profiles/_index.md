---
title: Analyze app performance using Pyroscope
menuTitle: Analyze app performance
description: How to use the Pyroscope UI to analyze performance of your applications.
weight: 40
aliases:
  - ../ingest-and-analyze-profile-data/analyze-profiles/
draft: true
keywords:
  - pyroscope
  - performance analysis
  - flamegraphs
---

<!-- This page is unpublished until we have more information. -->

# Analyze app performance using Pyroscope

Pyroscope's UI is designed to make it easy to visualize and analyze profiling data.
There are several different modes for viewing, analyzing, uploading, and comparing profiling data.
These modes are discussed in the [Pyroscope UI documentation]({{< relref "../pyroscope-ui" >}}).

![Screenshots of Pyroscope's UI](https://grafana.com/static/img/pyroscope/pyroscope-ui-diff-2023-11-30.png)

One of the major benefits of continuous profiling is the ability to compare and diff profiling data from two different queries:

- Comparing two different git commits before and after a code change
- Comparing Staging vs production environments to identify differences in performance
- Comparing performance between two different a/b tests or feature flag experiments
- Comparing memory allocations between two different time periods before and after a memory leak
- etc

With traditional profiling getting any of this information is much more difficult to organize, properly label, share, or store for later analysis. With Pyroscope, all of this is just a matter of writing the two queries you'd like to compare and clicking a button.

## Seamless integration with observability tools

![Flowchart showing Pyroscope integration with other tools](https://grafana.com/static/img/pyroscope/grafana-pyroscope-dashboard-2023-11-30.png)

Pyroscope enhances its value through seamless integration with leading observability tools like Grafana, Prometheus, and Loki. This integration facilitates deeper insights into application performance and aids in addressing issues identified through other monitoring methods.