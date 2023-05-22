---
title: "Grafana Phlare querier"
menuTitle: "Querier"
description: "The querier evaluates PromQL expressions."
weight: 50
---

# Grafana Phlare querier

The querier is a stateless component that evaluates queries  expressions by fetching profiles series and labels on the read path.

The querier uses the [ingester]({{< relref "ingester.md" >}}) component only to query recently written data. The support of querying the [long-term storage]({{< relref "../about-grafana-phlare-architecture/index.md#long-term-storage" >}}) is planned for the next release.

### Connecting to ingesters

You must configure the querier with the same `-ingester.ring.*` flags (or their respective YAML configuration parameters) that you use to configure the ingesters so that the querier can access the ingester hash ring and discover the addresses of the ingesters.

## Querier configuration

For details about querier configuration, refer to [querier]({{< relref "../../configure-server/reference-configuration-parameters/index.md#querier" >}}).
