---
aliases:
  - ../../visualizing-profiles/dashboards/reads-networking/
  - /docs/fire/latest/operators-guide/monitoring-grafana-fire/dashboards/reads-networking/
description: View an example Reads networking dashboard.
menuTitle: Reads networking
title: Grafana Fire Reads networking dashboard
weight: 100
---

# Grafana Fire Reads networking dashboard

The Reads networking dashboard shows receive and transmit bandwidth, in-flight requests, and TCP connections.
The dashboard isolates each service on the read path into its own section and displays the order in which a read request flows.

This dashboard requires [additional resources profiles]({{< relref "../../requirements.md#additional-resources-profiles" >}}).

## Example

The following example shows a Reads networking dashboard from a demo cluster.

![Grafana Fire reads networking dashboard](fire-reads-networking.png)
