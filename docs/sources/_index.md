---
title: "Grafana Phlare documentation"
menuTitle: "Grafana Phlare"
weight: 1
keywords:
  - Grafana Phlare
  - Grafana profiles
  - TSDB
  - profiles storage
  - profiles datastore
  - observability
---
<h1 align="center">Grafana Phlare documentation</h1>

[//TODO]: <> (Add logo once read)

<p align="center">Grafana Phlare is an open source software project for aggregating continuous profiling data. Continuous profiling is an
observability signal that allows you to understand your workload's resources (CPU, memory, etc...) usage down to the line number.</p>

Grafana Phlare is fully integrated with Grafana allowing you to **correlate** with other observability signals, like metrics, logs, and traces.

<video style="border-radius: 1%" autoplay loop>
  <source src="ui.webm" type="video/webm">
</video>

Core features of Grafana Phlare include:

- **Easy to install:** Using its monolithic mode, you can get Grafana Phlare up and
  running with just one binary and no additional dependencies. On Kubernetes a single helm chart
  allows to deploy in different mode.
- **Horizontal scalability:**  You can run Grafana Phlare
   across multiple machines, which makes it effortless for you to scale the database to handle the profiling volumes your workload generates.
- **High availability:** Grafana Phlare replicates incoming profiles, ensuring that
  no data is lost in the event of machine failure. This means you can rollout without
  interrupting profiles ingestion and analysis.
- **Cheap, durable profile storage:** Grafana Phlare uses object storage for long-term data storage,
  allowing it to take advantage of this ubiquitous, cost-effective, high-durability technology.
  It is compatible with multiple object store implementations, including AWS S3,
  Google Cloud Storage, Azure Blob Storage, OpenStack Swift, as well as any S3-compatible object storage.
- **Natively multi-tenant:** Grafana Phlare's multi-tenant architecture enables you
  to isolate data and queries from independent teams or business units, making it
  possible for these groups to share the same database.
