---
title: "Pyroscope querier"
menuTitle: "Querier"
description: "The querier evaluates Profiling queries."
weight: 50
---

# Pyroscope querier

The querier is a stateless component that evaluates query expressions by fetching profiles series and labels on the read path.

The querier uses the [ingesters]({{< relref "./ingester.md" >}}) for gathering recently written data and the [store-gateways] for the [long-term storage]({{< relref "../about-grafana-pyroscope-architecture/index.md#long-term-storage" >}}).

### Connecting to ingesters

You must configure the querier with the same `-ingester.ring.*` flags (or their respective YAML configuration parameters) that you use to configure the ingesters so that the querier can access the ingester hash ring and discover the addresses of the ingesters.

## Querier configuration

For details about querier configuration, refer to [querier]({{< relref "../../configure-server/reference-configuration-parameters/index.md#querier" >}}).
