---
aliases:
  - ../../visualizing-profiles/dashboards/slow-queries/
  - /docs/fire/latest/operators-guide/monitoring-grafana-fire/dashboards/slow-queries/
description: Review a description of the Slow queries dashboard.
menuTitle: Slow queries
title: Grafana Fire Slow queries dashboard
weight: 150
---

# Grafana Fire Slow queries dashboard

The Slow queries dashboard shows details about the slowest queries for a given time range and enables you to filter results by a specific tenant.

If you enable [Grafana Tempo](https://grafana.com/oss/tempo/) tracing, the dashboard displays a link to the trace of each query.

This dashboard requires [Grafana Loki](https://grafana.com/oss/loki/) to fetch detailed query statistics from logs.
