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

Code profiling itself is not new, but Continuous profiling is a much newwer, modern, approach to performance monitoring. Continuous profiling adds two new dimension to traditional profiles: 
- time: profiling data is collected continuously over time and can be queried at any point in time
- metadata: profiling data can be tagged with metadata to provide additional context about the data

Combining these two additional capabilities along with the already detail-rich nature of performance profiles makes continuous profiling a uniquely powerful tool. 

This is even further enhanced when combined with Pyroscope's powerful UI which allows you to slice and dice your profiles in a unique way that is not possible with traditional logs, metrics, or tracing tools. 

This walkthrough will not only show how Pyroscope functions similarly to other modern observability tools, providing a powerful, Prometheus-like experience for querying profiling data, but it will also it will also explain how you can leverage the unique capabilities of Pyroscope's UI to gain a deeper understanding of your application's performance.

![Infographic showing the synergy between continuous profiling and metadata](#placeholder)

## Key Features of the Pyroscope UI

### Tag Explorer Page

**Breaking Down Tags for Performance Insights**

The Tag Explorer page is a cornerstone of Pyroscope's UI, enabling users to dissect and analyze performance data based on tags or labels. This feature is instrumental in pinpointing anomalies and understanding how different segments of your application perform under various conditions.

![Screenshot or GIF of the Tag Explorer page](#placeholder)

### Single View Page

**In-Depth Analysis with Time and Label Filters**

The Single View page allows for a detailed examination of profiling data. Users can select specific time ranges and labels, offering a focused view on particular aspects of application performance. This is especially useful for diagnosing issues that occur under certain conditions or during specific time windows.

![Image or video demonstrating time and label filters on Single View page](#placeholder)

### Comparison Page

**Side-by-Side Flamegraph Comparisons**

The Comparison page is an innovative tool for conducting side-by-side analyses of different profiling data sets. By comparing flamegraphs for different label sets, users can visually contrast the performance impact of various features, environments, or releases. For instance, comparing feature flag A vs. B, or production vs. development environments, offers immediate visual insights into performance differences.

![Visuals showcasing flamegraph comparisons for different label sets](#placeholder)

### Diff Page

**Understanding Changes with Differential Analysis**

The Diff page is pivotal for understanding the changes between two sets of profiling data. It calculates and visualizes the differences in a normalized manner, focusing on the percentage of total time each function consumes. This tool is invaluable for quantifying the impact of changes and guiding optimization efforts.

![Detailed image or interactive visual of the Diff page](#placeholder)

## Why These Features Matter

**Making the Most of Pyroscope for Performance Optimization**

Each feature of Pyroscope's UI is designed to turn profiling data into actionable insights. The integration of continuous profiling with tag-based metadata transforms raw data into meaningful information, guiding developers towards effective performance optimization. Understanding the nuances of these features and their practical applications is crucial in leveraging Pyroscope to its full potential.

![Case study visuals or diagrams demonstrating Pyroscope UI features in use](#placeholder)

## Conclusion

**Empowering Developers with Advanced Profiling Tools**

Pyroscope's UI is more than just a display of profiling data; it's a suite of advanced tools designed to empower developers in diagnosing and resolving performance issues effectively. By utilizing these features, you can gain a comprehensive understanding of your application's performance, leading to more efficient and reliable software.

![Concluding graphic or call-to-action visual](#placeholder)
