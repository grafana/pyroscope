---
aliases:
  - ../../visualizing-profiles/dashboards/remote-ruler-reads-resources/
  - /docs/fire/latest/operators-guide/monitoring-grafana-fire/dashboards/remote-ruler-reads-resources/
description: View an example Remote ruler reads resources dashboard.
menuTitle: Remote ruler reads resources
title: Grafana Fire Remote ruler reads resources dashboard
weight: 110
---

# Grafana Fire Remote ruler reads resources dashboard

The Remote ruler reads resources dashboard shows CPU, memory, disk, and other resources utilization profiles for ruler query path components when remote operational mode is enabled.

The dashboard isolates each service on the ruler read path into its own section and displays the order in which a read request flows.

This dashboard requires [additional resources profiles]({{< relref "../../requirements.md#additional-resources-profiles" >}}).

## Example

The following example shows a Remote ruler reads resources dashboard from a demo cluster.

![Grafana Fire Remote ruler reads resources dashboard](fire-remote-ruler-reads-resources.png)
