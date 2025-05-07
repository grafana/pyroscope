---
headless: true
description: Shared file for Profiles Drilldown overview.
---

[//]: # 'This file documents an introduction to Profiles Drilldown.'
[//]: # 'This shared file is included in these locations:'
[//]: # '/pyroscope/docs/sources/configure-client/profile-types.md'
[//]: # '/pyroscope/docs/sources/introduction/profiling-types.md'
[//]: #
[//]: # 'If you make changes to this file, verify that the meaning and content are not changed in any place where the file is included.'
[//]: # 'Any links should be fully qualified and not relative: /docs/grafana/ instead of ../grafana/.'
<!-- Use Profiles Drilldown to investigate issues -->

{{< docs/public-preview product="Profiles Drilldown" >}}

[Grafana Profiles Drilldown](https://grafana.com/docs/grafana-cloud/visualizations/simplified-exploration/profiles/) is designed to make it easy to visualize and analyze profiling data.
There are several different modes for viewing, analyzing, and comparing profiling data.

The main use cases are the following:

- Proactive: Cutting costs, addressing latency issues, or optimizing memory usage for applications
- Reactive: Resolving incidents with line-level accuracy or debugging active latency/memory issues

Profiles Drilldown provides an intuitive interface to specifically support these use cases.
You get a holistic view of all of your services and how they're functioning, but also the ability to drill down for more targeted root cause analysis.

![Profiles Drilldown home screen](/media/docs/explore-profiles/explore-profiles-homescreen-v1.png)

Profiles Drilldown offers a convenient platform to analyze profiles and get insights that are impossible to get from using other traditional signals like logs, metrics, or tracing.

{{< youtube id="x9aPw_CbIQc" >}}

{{< docs/play title="the Grafana Play site" url="https://play.grafana.org/a/grafana-pyroscope-app/profiles-explorer" >}}

## Continuous profiling

While code profiling has been a long-standing practice, continuous profiling represents a modern and more advanced approach to performance monitoring.

This technique adds two critical dimensions to traditional profiles:

Time
: Profiling data is collected _continuously_, providing a time-centric view that allows querying performance data from any point in the past.

Metadata
: Metadata enriches profiling data, adding contextual depth to the performance data.

These dimensions, coupled with the detailed nature of performance profiles, make continuous profiling a uniquely valuable tool.

## Flame graphs

<!-- vale Grafana.We = NO -->

Flame graphs help you visualize resource allocation and performance bottlenecks, and you even get suggested recommendations and performance fixes via AI-driven flame graph analysis, as well as line-level insights from our GitHub integration.

<!-- vale Grafana.We = YES -->

On views with a flame graph, you can use **Explain flame graph** to provide an AI flame graph analysis that explains the performance bottleneck, root cause, and recommended fix.
For more information, refer to [Flame graph AI](https://grafana.com/docs/grafana-cloud/monitor-applications/profiles/flamegraph-ai/).