---
title: "Pyroscope v2 components"
menuTitle: "Components"
description: "Pyroscope v2 includes a set of components that interact to form a cluster."
weight: 30
keywords:
  - Pyroscope v2 components
  - Pyroscope distributor
  - Pyroscope segment-writer
  - Pyroscope metastore
  - Pyroscope compaction-worker
  - Pyroscope query-frontend
  - Pyroscope query-backend
---

# Pyroscope v2 components

Pyroscope v2 includes a set of components that interact to form a cluster.

Most components are stateless and don't require any data persisted between process restarts. The [metastore](metastore/) is the only stateful component in the architecture, using Raft consensus for replication and fault tolerance.

{{< section menuTitle="true" >}}
