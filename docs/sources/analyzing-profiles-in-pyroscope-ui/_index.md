---
title: "How to use the Pyroscope UI for performance analysis"
menuTitle: "How to use the Pyroscope UI"
description: "How to use the Pyroscope UI to analyze performance of your applications."
weight: 30
keywords:
  - pyroscope
  - UI
  - performance analysis
  - flamegraphs
---

# How to use the Pyroscope UI for performance analysis

## Introduction

**Understanding Continuous Profiling and Metadata in Pyroscope**

While code profiling has been a long-standing practice, Continuous Profiling represents a modern and more advanced approach to performance monitoring. This technique adds two critical dimensions to traditional profiles:

- **Time:** Profiling data is collected _continuously_, providing a time-centric view that allows querying performance data from any point in the past
- **Metadata:** Profiles are enriched with metadata, adding contextual depth to the performance data

These dimensions, coupled with the detailed nature of performance profiles, make Continuous Profiling a uniquely valuable tool. Pyroscope's UI enhances this further by offering a convenient platform to analyze profiles and get insights that are impossible to get from using other traditional signals like logs, metrics, or tracing. 

In this walkthrough, we'll show how Pyroscope parallels these other modern observability tools by providing a Prometheus-like querying experience. More importantly, you'll learn how to use Pyroscope's extensive UI features for a deeper insight into your application's performance.

[ TODO ] Add graphic (gif?) showing profiles enriched with of continuous profiling with time and metadata integration

## Key Features of the Pyroscope UI

### Single View Page

**Detailed Analysis with Various Viewing Options**

The Single View page in Pyroscope's UI is a hub for in-depth profile analysis. Here, you can explore a single flamegraph with multiple viewing options:

- **Table View:** Breaks down the profiling data into a sortable table format.
- **Sandwich View:** Displays both the callers and callees for a selected function, offering a comprehensive view of function interactions.
- **Flamegraph View:** Visualizes profiling data in a flamegraph format, allowing easy identification of resource-intensive functions.
- **Export & Share:** Options to export the flamegraph for offline analysis or share it via a flamegraph.com link for collaborative review.

**Real-World Example:** Imagine analyzing a Python application's memory usage. You notice a gradual increase in memory consumption. Using the Single View, you could examine a memory profile at a specific point in time, switch to the Sandwich View to understand the relationship between functions, and then share this with your team for a collaborative diagnosis.

**Visual Placeholder:** *Screenshots demonstrating each view option in the Single View page.*

### Comparison Page

**Conducting Comparative Analysis with Label Sets**

The Comparison page facilitates side-by-side comparison of profiles based on different label sets. This feature is invaluable for understanding the impact of changes or differences between environments.

**How to Compare:**
1. Select two different sets of labels (e.g., `env:production` vs. `env:development`).
2. View the resulting flamegraphs side by side to identify disparities in performance.

**Examples of Comparative Analysis:**
- **Feature Flags:** Compare application performance with `feature_flag:a` vs. `feature_flag:b`.
- **Deployment Environments:** Contrast `env:production` vs. `env:development`.
- **Release Analysis:** Examine `commit:release-1` vs. `commit:release-2`.

**Visual Placeholder:** *Graphics showing the comparison of two different label sets.*

### Diff Page

**Identifying Changes with Differential Analysis**

The Diff page is crucial for understanding the differences between two profiling data sets. It normalizes the data by comparing the percentage of total time spent in each function.

**Real-World Use Case:**
- **Performance Optimization:** After implementing a performance optimization, compare the profiles before and after the change to quantify the improvement.
- **Memory Leak Diagnosis:** Compare memory profiles from before the leak was noticed to after, identifying the functions contributing to the leak.

**How to Read the Diff:**
- Understand that the diff highlights the change in resource consumption between two profiles, with a focus on relative differences.
- Look for significant discrepancies in function time percentages, which indicate areas of change.

**Visual Placeholder:** *Annotated visuals explaining how to interpret the differential analysis on the Diff page.*

---

This enhanced document provides a thorough guide on using Pyroscope's UI for detailed application performance analysis. It includes real-world examples and practical tips to help users effectively leverage the platform's capabilities.