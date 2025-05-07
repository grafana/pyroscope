---
headless: true
description: Shared file for intro to flame graphs.
---

[//]: # 'Learn about flame graphs.'
[//]: # 'This shared file is included in these locations:'
[//]: # '/pyroscope/docs/sources/introduction/flamegraphs.md'
[//]: # '/website/content/grafana-cloud/monitor-applications/profiles/introduction/flamegraphs.md'
[//]: # '/explore-profiles/docs/sources/introduction/flame-graphs.md'
[//]: # 'If you make changes to this file, verify that the meaning and content are not changed in any place where the file is included.'
[//]: # 'Any links should be fully qualified and not relative: /docs/grafana/ instead of ../grafana/.'

<!-- Flame graphs: Visualizing performance data -->

Flame graphs provide a visual summary of your profile data.
A flame graph is a complete visualization of hierarchical data, for example stack trace and, file system contents, with a metric, typically resource usage, attached to the data.

A fundamental aspect of continuous profiling is the flame graph, a convenient way to visualize performance data.
These graphs provide a clear, intuitive understanding of resource allocation and bottlenecks within the application.

<!-- vale Grafana.Spelling = NO -->

[Brendan Gregg](https://www.brendangregg.com/flamegraphs.html), the creator of flame graphs, was inspired by the inability to view, read, and understand stack traces using the regular profilers to debug performance issues.

<!-- vale Grafana.Spelling = YES -->

## How Pyroscope creates flame graphs

This diagram shows how code is turned into a flame graph.
In this case, Pyroscope samples the stacktrace of your application to understand how many CPU cycles are spent in each function.
It then aggregates this data and turns it into a flame graph.

![code to flame graph diagram](https://grafana.com/static/img/pyroscope/code-to-flamegraph-animation.gif)

## What does a flame graph represent?

Horizontally, the flame graph represents 100% of the time that this application was running.
The width of each node represents the amount of time spent in that function.
The wider the node, the more time spent in that function. The narrower the node, the less time spent in that function.

Vertically, the nodes in the flame graph represent the hierarchy of functions called and time spent in each function.
The top node is the root node and represents the total amount of time spent in the application.
The nodes below it represent the functions called and time spent in each function.
The nodes below those represent the functions called from those functions and time spent in each function.
This continues until you reach the bottom of the flame graph.

This is a CPU profile, but profiles can represent many other types of resource such as memory, network, disk, etc.

![flame graph](https://grafana.com/static/img/pyroscope/pyroscope-flamegraph-2023-11-30.png)

## Flame graph visualization panel UI

To learn more about the flame graph user interface in Grafana, Grafana Cloud, and Grafana Profiles Drilldown, refer to [Flame graph visualization panel](https://grafana.com/docs/grafana-cloud/visualizations/panels-visualizations/visualizations/flame-graph).

To learn more about the flame graph in the Pyroscope UI, refer to [Pyroscope UI](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/view-and-analyze-profile-data/pyroscope-ui/).