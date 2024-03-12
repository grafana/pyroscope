---
title: View and analyze profile data
menuTitle: View and analyze profile data
description: How to use Pyroscope to view and analyze profile data.
aliases:
  - ../ingest-and-analyze-profile-data/
weight: 50
keywords:
  - pyroscope
  - UI
  - performance analysis
  - flamegraphs
  - CLI
---

# View and analyze profile data

Profiling data can be presented in a variety of formats presents such as:
- **Flamegraphs**: Visualize call relationships and identify hotspots.
- **Tables**: View detailed statistics for specific functions or time periods.
- **Charts and graphs**: Analyze trends and compare performance across different metrics.

## Viewing profiles

Pyroscope offers both a Command Line Interface (CLI) and an Application Programming Interface (API) to interact with and retrieve profiling data. These tools provide flexibility in how you access and manage your profiling information.

You can export profiling data from Pyroscope in various formats:
- **pprof**: Support for pprof, gzip compressed pprof, for example, `foo.pprof.gz`
- **JSON**: JSON object easy to integrate with other tools and scripts

Integrating Pyroscope with Grafana is a common and recommended approach for visualizing profiling data. Grafana, being a powerful tool for data visualization, can effectively display profiling data in an accessible and insightful manner.

Options for visualizing data in Grafana:

- **Pyroscope App Plugin**: This plugin is specifically designed for Pyroscope data. It allows for easy browsing, analysis, and comparison of multiple profiles across different labels or time periods. This is particularly useful for a comprehensive overview of your application's performance.
- **Explore tab**: In Grafana, **Explore** is suited for making targeted queries on your profiling data. This is useful for in-depth analysis of specific aspects of your application's performance.
- **Dashboard**: Grafana dashboards are excellent for integrating profiling data with other metrics. You can display Pyroscope data alongside other dashboard items, creating a unified view of your application’s overall health and performance.

For more information on using profiles in Grafana, refer to [Pyroscope and profiles in Grafana]({{< relref "../introduction/pyroscope-in-grafana#pyroscope-and-profiling-in-grafana" >}}).

The Pyroscope app plugin works for Grafana Cloud.

For more information on configuring these data sources, refer to the Pyroscope data source documentation in [Grafana Cloud](/docs/grafana-cloud/connect-externally-hosted/data-sources/grafana-pyroscope/) and [Grafana](/docs/grafana/latest/datasources/grafana-pyroscope/).
