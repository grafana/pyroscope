---
title: "Reference: Grafana Phlare glossary"
menuTitle: "Reference: Glossary"
description: "Grafana Phlare glossary terms."
weight: 130
---

# Reference: Grafana Phlare glossary

## Blocks storage

Blocks storage is the Phlare storage engine based on the Prometheus TSDB.
Grafana Phlare stores blocks in object stores such as AWS S3, Google Cloud Storage (GCS), Azure blob storage, or OpenStack Object Storage (Swift).
For a complete list of supported backends, refer to [About the architecture]({{< relref "architecture/about-grafana-phlare-architecture/index.md" >}})

## Churn

Churn is the frequency at which series become idle.

A series becomes idle once it's no longer exported by the monitored targets.
Typically, series become idle when a monitored target process or node gets terminated.

## Component

Grafana Phlare comprises several components.
Each component provides a specific function to the system.
For component specific documentation, refer to one of the following topics:

- [Distributor]({{< relref "architecture/components/distributor.md" >}})
- [Ingester]({{< relref "architecture/components/ingester.md" >}})
- [Querier]({{< relref "architecture/components/querier.md" >}})

## Flushing

Flushing is the operation run by ingesters to offload profiles from memory and store them in the long-term storage.

## Gossip

Gossip is a protocol by which components coordinate without the need for a centralized entity. Peer-to-peer communication is used to disseminate information to all the members in a cluster.

## Hash ring

The hash ring is a distributed data structure used by Grafana Phlare for sharding, replication, and service discovery.
Components use a [memberlist]({{< relref "#memberlist" >}}) cluster to share the hash ring data structure.
For more information, refer to the [Hash ring]({{< relref "architecture/hash-ring/index.md" >}}).

## Memberlist

Memberlist manages cluster membership and member failure detection using [gossip]({{< relref "#gossip" >}}).

## Org

Refer to [Tenant]({{< relref "#tenant" >}}).

## Ring

Refer to [Hash ring]({{< relref "#hash-ring" >}}).

## Profile

A profile or performance profile is a set of samples collected from a running process.
The sample data is used to identify the source of performance issues in the process.
Samples usually contain information about the [stacktrace](#Stacktrace), CPU usage, memory usage, and other metrics.

## Series

A series is a single stream of [profiles]({{< relref "#profile" >}}) belonging to the same process, with the same set of label key-value pairs.

Given a single profile `profile_cpu` you may have multiple series, each one uniquely identified by the combination of metric name and unique label key-value pairs:

```
profile_cpu{instance="10.0.0.1",pod="pod-a",namespace="prod"}
profile_cpu{instance="10.0.0.1",pod="pod-b",namespace="prod"}
profile_cpu{instance="10.0.0.2",pod="pod-a",namespace="dev"}
```

## Tenant

A tenant is the owner of a set of profile series written to and queried from Grafana Phlare.
Grafana Phlare isolates profile series belonging to different tenants.

## Profiles series

Refer to [Series]({{< relref "#series" >}}).

## User

Refer to [Tenant]({{< relref "#tenant" >}}).


## PProf

pprof is a tool for the visualization and analysis of profiling data. We often refer to pprof as
the wire [protocol](https://github.com/google/pprof/blob/main/proto/profile.proto) used by Phlare for pushing profiles.

## Stacktrace

A stack trace is a unique sequence of nested functions called by a program.

## flamegraph

[Flame graphs](https://www.brendangregg.com/flamegraphs.html) are a visualization of hierarchical data,
created to visualize [stack traces](#Stacktrace) of profiled software so that the most frequent code-paths
can be identified quickly and accurately.

Flamegraph when inverted (deepest stack frames at the bottom) are called Icicle Graphs.


## top table

In the top table, you can see the top functions that are consuming the most resources.
