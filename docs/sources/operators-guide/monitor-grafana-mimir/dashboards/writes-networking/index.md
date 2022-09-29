---
aliases:
  - ../../visualizing-profiles/dashboards/writes-networking/
  - /docs/fire/latest/operators-guide/monitoring-grafana-fire/dashboards/writes-networking/
description: View an example Writes networking dashboard.
menuTitle: Writes networking
title: Grafana Fire Writes networking dashboard
weight: 190
---

# Grafana Fire Writes networking dashboard

The Writes networking dashboard shows receive/transmit bandwidth, inflight requests, and TCP connections.
The dashboard isolates each service on the write path into its own section and displays the order in which a write request flows.

This dashboard requires [additional resources profiles]({{< relref "../../requirements.md#additional-resources-profiles" >}}).

## Example

The following example shows a Writes networking dashboard from a demo cluster.

![Grafana Fire writes networking dashboard](fire-writes-networking.png)
