---
title: Analyze app performance using Pyroscope  
menuTitle: Analyze app performance
description: How to use the Pyroscope UI to analyze performance of your applications.
weight: 30
keywords:
  - pyroscope
  - UI
  - performance analysis
  - flamegraphs
---

# Analyze app performance using Pyroscope

## Introduction

##  Continuous profiling and metadata 

While code profiling has been a long-standing practice, Continuous Profiling represents a modern and more advanced approach to performance monitoring. This technique adds two critical dimensions to traditional profiles:

- **Time:** Profiling data is collected _continuously_, providing a time-centric view that allows querying performance data from any point in the past
- **Metadata:** Profiles are enriched with metadata, adding contextual depth to the performance data

These dimensions, coupled with the detailed nature of performance profiles, make Continuous Profiling a uniquely valuable tool. Pyroscope's UI enhances this further by offering a convenient platform to analyze profiles and get insights that are impossible to get from using other traditional signals like logs, metrics, or tracing. 

In this walkthrough, we'll show how Pyroscope parallels these other modern observability tools by providing a Prometheus-like querying experience. More importantly, you'll learn how to use Pyroscope's extensive UI features for a deeper insight into your application's performance.

[ TODO ] Add graphic (gif?) showing profiles enriched with of continuous profiling with time and metadata integration

## Key Features of the Pyroscope UI

### Tag Explorer Page

The Tag Explorer page is a vital part of Pyroscope's UI, allowing users to navigate and analyze performance data through tags/labels. This feature is crucial for identifying performance anomalies and understanding the behavior of different application segments under various conditions. We intentionally don't include a query language on this page as we built this page to be as intuitive as possible for users to use the UI to navigate and drill down into which tags are most interesting to them.

To use the Tag Explorer:
1. Select a tag to view the corresponding profiling data
2. Analyze the pie chart and the table of descriptive statsitcs to determine which tags if any are behaving abnormally
3. Select a tag to view the corresponding profiling data
4. Make use of the shortcuts to the single, comparison, and diff pages to further identify the root cause of the performance issue

[TODO] A screenshot or GIF of the tag explorer page, possibly labeled with numbers to show the steps above

### Single View Page

The Single View page in Pyroscope's UI is built for in-depth profile analysis. Here, you can explore a single flamegraph with multiple viewing options and functionalities:

- **Table View:** Breaks down the profiling data into a sortable table format
- **Sandwich View:** Displays both the callers and callees for a selected function, offering a comprehensive view of function interactions
- **Flamegraph View:** Visualizes profiling data in a flamegraph format, allowing easy identification of resource-intensive functions
- **Export & Share:** Options to export the flamegraph for offline analysis or share it via a flamegraph.com link for collaborative review

**Visual Placeholder:** *Screenshots demonstrating each view option in the Single View page.*

In the picture above we see a spike in CPU usage. Without profiling we would go from a memory spike to digging through code or guessing what the cause of it is. However, with profiling we can use the flamegraph and table to see exactly which function is most responsible for the spike. Often this will show up as a single node taking up a noticeably disproportionate width in the flamegraph as seen below with the "checkDriverAvailability" function.

[ picture ]

However, in some instances it may be a function that is called many times and is taking up a large amount of space in the flamegraph. In this case we can use the sandwich view to see that a logging function called throughout many functions in the codebase is the culprit. To learn more about how to read the sandwich view check out or [flamegraph 101 documentation].

[ picture ]

### Comparison Page

**Conducting Comparative Analysis with Label Sets**

The Comparison page facilitates side-by-side comparison of profiles either based on different label sets, different time periods, or both. This feature is extremely valuable for understanding the impact of changes or differences between do distinct queries of your application. 

**How to Compare:**
1. Select two different sets of labels (e.g., `env:production` vs. `env:development`) and or time periods, reflected by the sub-timelines above each flamegraph
2. View the resulting flamegraphs side by side to identify disparities in performance

**Examples of Comparative Analysis:**
We see many practical use cases for comparison for companies using Pyroscope. Some examples of labels below experessed as `label:value` are:
- **Feature Flags:** Compare application performance with `feature_flag:a` vs. `feature_flag:b`
- **Deployment Environments:** Contrast `env:production` vs. `env:development`
- **Release Analysis:** Examine `commit:release-1` vs. `commit:release-2`
- **Region:** Compare `region:us-east-1` vs. `region:us-west-1`

[Image showing the comparison of two different label sets]

Another example whre time is more important than labels is when you want to compare two different time periods. For example, in investigating the cause of a memory leak you would see something like the following where the timeline shows an steadily increasing amount of memory allocations over time. This is a clear indicator of a memory leak. 

You can then use the comparison page to compare the memory allocations between two different time periods where allocations were low and where allocations were high which would allow you to identify the function that is causing the memory leak.

[ picture ]

### Diff Page

**Identifying Changes with Differential Analysis**

The Diff page is realy an extension of the comparison page, crucial for more easily visually showing the differences between two profiling data sets. It normalizes the data by comparing the percentage of total time spent in each function so that the resulting flamegraph is comparing the __share__ of time spent in each function rather than the absolute amount of time spent in each function. This is important because it allows you to compare two different queries that may have different total amounts of time spent in each function.

Similar to a git diff it takes the flamegraphs from the comparison page and highlights the differences between the two flamegraphs where red represents an increase in cpu usage from the baseline to the comparison and green represents a decrease.

Using the same examples from above here is a diff between two label sets:
[Image showing the comparison __diff__ of two different label sets]

and a diff between two time periods during a introduction of a memory leak:
[Image showing the comparison __diff__ of two different time periods]

### Using Pyroscope within Grafana

One of the major benefits of Pyroscope is that it can be used alongside the other Grafana tools such as Loki, Tempo, Mimir, and k6. This allows you to use Pyroscope to get the most granular insight into your application and how you can use it to fix issues that you may have identified via metrics, logs, traces, or anything else.

You can use Pyroscope within Grafana by using the Pyroscope datasource plugin. This plugin allows you to query Pyroscope data from within Grafana and visualize it alongside your other Grafana data. 

For example here is a screenshot of the explore page where we've combined traces and profiles to be able to see granular line-level detail when available for a trace span. This allows you to see the exact function that is causing a bottleneck in your application as well as a specific request.

[ picture ]

And here is an example of how you can integrate profiles into your dashboards. In this case we showing memory profiles alongside panels for logs and metrics to be able to debug OOM errors alongside the associated logs and metrics.

[ picture ]