---
title: Use the Pyroscope UI to explore profiling data
menuTitle: Use the Pyroscope UI
description: How to use the Pyroscope UI to explore profile data.
weight: 40
aliases:
  - ../ingest-and-analyze-profile-data/profile-ui/
keywords:
  - pyroscope
  - performance analysis
  - flame graphs
---

# Use the Pyroscope UI to explore profiling data

Pyroscope's UI is designed to make it easy to visualize and analyze profiling data.
There are several different modes for viewing, analyzing, uploading, and comparing profiling data.

![Screenshot of Pyroscope UI](/media/docs/pyroscope/screenshot-pyroscope-comparison-view.png)

While code profiling has been a long-standing practice, continuous profiling represents a modern and more advanced approach to performance monitoring. This technique adds two critical dimensions to traditional profiles:

Time
: Profiling data is collected _continuously_, providing a time-centric view that allows querying performance data from any point in the past.

Metadata
: Profiles are enriched with metadata, adding contextual depth to the performance data.

These dimensions, coupled with the detailed nature of performance profiles, make continuous profiling a uniquely valuable tool.
Pyroscope's UI enhances this further by offering a convenient platform to analyze profiles and get insights that are impossible to get from using other traditional signals like logs, metrics, or tracing.

In this UI reference, you'll learn how Pyroscope parallels these other modern observability tools by providing a Prometheus-like querying experience. More importantly, you'll learn how to use Pyroscope's extensive UI features for a deeper insight into your application's performance.

## Key features of the Pyroscope UI

The following sections describe Pyroscope UI capabilities.

<!-- Add a screenshot with numbered parts for each of the sections described below. -->

## Tag Explorer

The **Tag Explorer** page lets you navigate and analyze performance data through tags and labels.
This feature is crucial for identifying performance anomalies and understanding the behavior of different application segments under various conditions.
Pyroscope intentionally doesn't include a query language on this page.

![Pyroscope Tag Explorer](/media/docs/pyroscope/screenshot-pyroscope-tag-explorer.png)

To use the **Tag Explorer**:

1. Select a tag to view the corresponding profiling data.
1. Analyze the pie chart and the table of descriptive statistics to determine which tags if any are behaving abnormally.
1. Select a tag to view the corresponding profiling data.
1. Make use of the shortcuts to the Single, Comparison, and Diff View pages to further identify the root cause of the performance issue.

## Single view

The Single View page in Pyroscope's UI is built for in-depth profile analysis. Here, you can explore a single flame graph with multiple viewing options and functionalities:

**Table view**
: Breaks down the profiling data into a sortable table format. Selecting **Top Table** displays the table and hides the flame graph.

**Sandwich view**
: Displays both the callers and callees for a selected function, offering a comprehensive view of function interactions. Access by clicking in the flame graph and selecting **Sandwhich view**.

**Flame Graph** view
: Visualizes profiling data in a flame graph format, allowing easy identification of resource-intensive functions. Selecting **Flame Graph** displays the flame graph and hides the table.

**Both** view
: Displays both the table and the flame graph. This is the default view for **Single View**.

**Export Data**
: Options to export the flame graph for offline analysis or share it via a flamegraph.com link for collaborative review.

<!-- Visual Placeholder:** *Screenshots demonstrating each view option in the Single View page.* -->

Let's say that your app has a spike in memory usage.
Without profiling, you would go from a memory spike to digging through code or guessing the cause.
However, with profiling, you can use the flame graph and table to see exactly which function is most responsible for the spike.
Often, this shows up as a single node taking up a noticeably disproportionate width in the flame graph as seen below with the `checkDriverAvailability` function.

![example-flamegraph](https://grafana.com/static/img/pyroscope/pyroscope-ui-single-2023-11-30.png)

However, in some instances it may be a function that's called many times and is taking up a large amount of space in the flame graph.
In this case, you can use the sandwich view to see that a logging function called throughout many functions in the codebase is the culprit.

![example-sandwich-view](https://grafana.com/static/img/pyroscope/sandwich-view-2023-11-30.png)

### Comparison page

The Comparison page facilitates side-by-side comparison of profiles either based on different label sets, different time periods, or both.
This feature is extremely valuable for understanding the impact of changes or differences between do distinct queries of your application.

![Pyroscope Comparison view](/media/docs/pyroscope/screenshot-pyroscope-comparison-view.png)

To run a comparison:

1. Select two different sets of labels (for example, `env:production` vs. `env:development`) and or time periods, reflected by the sub-timelines above each flame graph.
1. View the resulting flame graphs side by side to identify disparities in performance.

There are many practical use cases for comparison for companies using Pyroscope.
Some examples of labels below expressed as `label:value` are:

Feature flags
: Compare application performance with `feature_flag:a` vs. `feature_flag:b`

Deployment environments
: Contrast `env:production` vs. `env:development`

Release analysis
: Examine `commit:release-1` vs. `commit:release-2`

Region
: Compare `region:us-east-1` vs. `region:us-west-1`

Another example where time is more important than labels is when you want to compare two different time periods.
For example, in investigating the cause of a memory leak you would see something like the following where the timeline shows a steadily increasing amount of memory allocations over time. This is a clear indicator of a memory leak.

You can then use the comparison page to compare the memory allocations between two different time periods where allocations were low and where allocations were high which would allow you to identify the function that's causing the memory leak.

![comparison-ui](https://grafana.com/static/img/pyroscope/pyroscope-ui-comparison-2023-11-30.png)

## Diff page: Identify changes with differential analysis

The **Diff** page is an extension of the comparison page, crucial for more easily visually showing the differences between two profiling data sets.
It normalizes the data by comparing the percentage of total time spent in each function so that the resulting flame graph is comparing the __share__ of time spent in each function rather than the absolute amount of time spent in each function.
This is important because it allows you to compare two different queries that may have different total amounts of time spent in each function.

![Diff view in Pyroscope](/media/docs/pyroscope/screenshot-pyroscope-diff-view.png)

Similar to a `git diff`, it takes the flame graphs from the comparison page and highlights the differences between the two flame graphs where red represents an increase in CPU usage from the baseline to the comparison and green represents a decrease.

<!-- and a diff between two time periods during an introduction of a memory leak:
![memory leak](https://grafana.com/static/img/pyroscope/pyroscope-memory-leak-2023-11-30.png) -->
