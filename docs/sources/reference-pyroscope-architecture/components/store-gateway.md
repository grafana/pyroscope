---
title: "Pyroscope Store-gateway"
menuTitle: "Store-gateway"
description: "The store-gateway retrieves profiling data from long-term storeage."
weight: 55
---

# Pyroscope Store-gateway

The store-gateways in Pyroscope are responsible for looking up profiling data in the [long-term storage]({{< relref "../about-grafana-pyroscope-architecture/index.md#long-term-storage" >}}) bucket. A single store-gateway is responsible for a subset of the blocks in the long-term storage and will be involved by a [querier].

## Store-gateway configuration

For details about store-gateway configuration, refer to [store-gateway]({{< relref "../../configure-server/reference-configuration-parameters/index.md#store_gateway" >}}).
