---
aliases:
  - ../../visualizing-metrics/dashboards/remote-ruler-reads-resources/
  - /docs/mimir/latest/operators-guide/monitoring-grafana-mimir/dashboards/remote-ruler-reads-resources/
description: View an example Remote ruler reads resources dashboard.
menuTitle: Remote ruler reads resources
title: Grafana Mimir Remote ruler reads resources dashboard
weight: 110
---

# Grafana Mimir Remote ruler reads resources dashboard

The Remote ruler reads resources dashboard shows CPU, memory, disk, and other resources utilization metrics for ruler query path components when remote operational mode is enabled.

The dashboard isolates each service on the ruler read path into its own section and displays the order in which a read request flows.

This dashboard requires [additional resources metrics]({{< relref "../../requirements.md#additional-resources-metrics" >}}).

## Example

The following example shows a Remote ruler reads resources dashboard from a demo cluster.

![Grafana Mimir Remote ruler reads resources dashboard](mimir-remote-ruler-reads-resources.png)
