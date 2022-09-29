---
title: "Grafana Fire documentation"
menuTitle: "Grafana Fire"
weight: 1
keywords:
  - Grafana Fire
  - Grafana profiles
  - TSDB
  - profiles storage
  - profiles datastore
  - observability
---
# Grafana Fire documentation

[//TODO]: <> (Add logo once read)

<p align="center">Grafana Fire is an open source software project for aggregating continuous profiling data. Continuous profiling is
observability signal allowing you to understand your workload's resources (CPU, memory, etc...) usage down to the line number.</p>

Grafana Fire fully integrated with Grafana allowing you to **correlate** with other observability signals.

Some core features of Grafana Fire includes:

- **Easy to install:** Using its monolithic mode, you can get Grafana Fire up and
  running with just one binary and no additional dependencies. On Kubernetes a single helm chart
  allows to deploy in different mode.
- **Horizontal scalability:**  You can run Grafana Fire's horizontally-scalable
  architecture across multiple machines, to accommodate to the volume of workload analyzed.
- **High availability:** Grafana Fire replicates incoming profiles, ensuring that
  no data is lost in the event of machine failure. Meaning you can rollout without
  interrupting profiles ingestion and analysis.
- **Cheap durable profiles:** Grafana Fire uses object storage for long-term data storage,
  allowing it to take advantage of this ubiquitous, cost-effective, high-durability technology.
  It is compatible with multiple object store implementations, including AWS S3,
  Google Cloud Storage, Azure Blob Storage, OpenStack Swift, as well as any S3-compatible object storage.
- **Natively multi-tenant:** Grafana Fire's multi-tenant architecture enables you
  to isolate data and queries from independent teams or business units, making it
  possible for these groups to share the same cluster.

```
TODO: This is gonna be removed once complete

# Operator and user guide

> Get started + video = Cyril
> Deploy on Kubernetes = Christian
> Architecture = Christian
  > About the architecture
  > Deployment mode
  > Components
  > Block Format
  > Hash rings
  > Memberlist and gossip protocol
> Configuration = Cyril
  > About configuration (File vs Args / config map rolling out...)
  > Configure on disk storage
  > Configure object storage
  > About tenant id
  > Configuring memberlist (+DNS service discovery)
  > About Grafana Fire IP address logging of a reverse proxy
  > Configuration parameters (all of it)
  > Configure Tracing
> Configure agent
    > About the agent (pull mode and only pprof)
    > Language support (build demo)
      > Go
      > Rust = Christian
      > Python = Christian
      > JVM = Cyril
      > NodeJS = Cyril
    > Coming soon Grafana Agent support
    > System wide and ebpf (coming soon)
> Monitor Fire (coming soon)
> Run in production (coming soon)
> Reference HTTP API (coming soon unstable talk about connect)
> Reference Glossary
> Learning resources (blogposts, play.grafana.org)
```
