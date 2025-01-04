---
title: Traces to profiles
menuTitle: Traces to profiles
description: Learn about traces to profiles integration in Grafana and Grafana Cloud.
weight: 500
keywords:
  - pyroscope
  - continuous profiling
  - tracing
aliases:
  - ./profile-tracing/traces-to-profiles/ # https://grafana.com/docs/pyroscope/latest/view-and-analyze-profile-data/profile-tracing/traces-to-profiles/
---

# Traces to profiles

{{< admonition type="note" >}}

Your application must be instrumented for profiles and traces. For more information, refer to [Link traces to profiles](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/trace-span-profiles/).

{{< /admonition >}}

[//]: # 'Shared content for Trace to profiles in the Tempo data source'

{{< docs/shared source="grafana" lookup="datasources/tempo-traces-to-profiles.md" version="<GRAFANA VERSION>" >}}
