# <p align="center">Grafana Phlare</p>

<p align="center"><img src="images/logo.png" alt="Grafana Phlare logo" width="400"></p>


<p align="center">Grafana Phlare is an open source software project for aggregating continuous profiling data. Continuous profiling is an
observability signal that allows you to understand your workload's resources (CPU, memory, etc...) usage down to the exact lines of code.</p>

Grafana Phlare is fully integrated with Grafana allowing you to **correlate** with other observability signals, like metrics, logs, and traces.

<p align="center">
  <img alt="Explore UI" src=images/grafana-profiles.gif>
</p>

[If you want to understand what profiling data looks like, try the profiling experience in play.grafana.com]()

[//TODO]: <> (link to live demo/play.grafana.com.)

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

## Deploying Grafana Phlare

For information about how to deploy Grafana Phlare, refer to [Deploy Grafana Phlare](https://grafana.com/docs/phlare/latest/operators-guide/deploying-grafana-phlare/).

## Getting started

If you’re new to Grafana Phlare, read the [Getting started guide](https://grafana.com/docs/phlare/latest/operators-guide/getting-started/).

Before deploying Grafana Phlare in a production environment, read:

1. [An overview of Grafana Phlare’s architecture](https://grafana.com/docs/phlare/latest/operators-guide/architecture/)
1. [Configure Grafana Phlare](https://grafana.com/docs/phlare/latest/operators-guide/configuring/)
1. [Deploy in Kubernetes](https://grafana.com/docs/phlare/latest/operators-guide/deploy-kubernetes/)

## Documentation

Refer to the following links to access Grafana Phlare documentation:

- [Latest release](https://grafana.com/docs/phlare/latest/)
- [Upcoming release](https://grafana.com/docs/phlare/next/), at the tip of the main branch

## Contributing

To contribute to Grafana Phlare, refer to [Contributing to Grafana Phlare](https://github.com/grafana/phlare/tree/main/docs/internal/contributing).

## Join the Grafana Phlare discussion

If you have any questions or feedback regarding Grafana Phlare, join the [Grafana Phlare Discussion](https://github.com/grafana/phlare/discussions). Alternatively, consider joining the monthly [Grafana Phlare Community Call](TODO-doc-link).

Your feedback is always welcome, and you can also share it via the [`#phlare` Slack channel](https://grafana.slack.com/archives/C047CCW6YM8).

## License

Grafana Phlare is distributed under [AGPL-3.0-only](LICENSE).
