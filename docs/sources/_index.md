---
title: "Grafana Pyroscope documentation"
menuTitle: "Grafana Pyroscope"
weight: 1
description: "Grafana Pyroscope documentation"
keywords:
  - Grafana Pyroscope
  - Grafana profiles
  - TSDB
  - profiles storage
  - profiles datastore
  - observability
  - continuous profiling
---
# Grafana Pyroscope documentation

![Grafana Pyroscope](logo.png)

Grafana Pyroscope is an open source software project for aggregating continuous profiling data.
Continuous profiling is an observability signal that allows you to understand your workload's resources (CPU, memory, etc.) usage down to the line number.

Grafana Pyroscope is fully integrated with Grafana allowing you to **correlate** with other observability signals, like metrics, logs, and traces.

## Continuous profiling

Continuous profiling is more than just a performance analysis tool.
It's a crucial component in modern software development and operations.
It goes past traditional profiling techniques by providing ongoing, in-depth insights into application performance.

Continuous profiling goes past the ephemeral, localized nature of traditional profiling (which historically has been more similar to `console.log` or `print statement` debugging) to a structured, centralized approach allows for effective use in production environments.
Put more simply, Pyroscope takes you from a bunch of flamegraph files on your desktop to a database where you can query and analyze production data in a structured way.

{{% admonition type="tip" %}}
To learn more about continuous profiling, read [the Introduction section]({{< relref "./introduction" >}}).
{{% /admonition %}}

Pyroscope, in particular, offers the flexibility to either visualize more traditional adhoc data or evolve your applications observability tooling to include more modern continuous profiling capabilities.


<video style="border-radius: 1%" autoplay loop>
  <source src="ui.webm" type="video/webm">
</video>

## Features

Core features of Grafana Pyroscope include:

- **Easy to install:** Using its monolithic mode, you can get Grafana Pyroscope up and
  running with just one binary and no additional dependencies. On Kubernetes, a single helm chart
  allows for deploying in different modes.
- **Horizontal scalability:**  You can run Grafana Pyroscope
   across multiple machines, which makes it effortless for you to scale the database to handle the profiling volumes your workload generates.
- **High availability:** Grafana Pyroscope replicates incoming profiles, ensuring that
  no data is lost in the event of machine failure. This means you can rollout without
  interrupting profiles ingestion and analysis.
- **Cheap, durable profile storage:** Grafana Pyroscope uses object storage for long-term data storage,
  allowing it to take advantage of this ubiquitous, cost-effective, high-durability technology.
  It is compatible with multiple object store implementations, including AWS S3,
  Google Cloud Storage, Azure Blob Storage, OpenStack Swift, as well as any S3-compatible object storage.
- **Natively multi-tenant:** Grafana Pyroscope's multi-tenant architecture enables you
  to isolate data and queries from independent teams or business units, making it
  possible for these groups to share the same database.

## Get started with Pyroscope

Begin your journey with Pyroscope. Visit our [Getting Started Guide](/docs/pyroscope/latest/get-started/) to learn about the different ways to instrument your application with Pyroscope. Join our [Slack community](https://grafana.slack.com/archives/C049PLMV8TB) and contribute to the evolving world of continuous profiling.
