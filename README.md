# <p align="center">Grafana Fire</p>

[//TODO]: <> (Add logo once read)

<p align="center">Grafana Fire is an open source software project for aggregating continuous profiling data. Continuous profiling is an
observability signal that allows you to understand your workload's resources (CPU, memory, etc...) usage down to the line number.</p>

Grafana Fire is fully integrated with Grafana allowing you to **correlate** with other observability signals, like metrics, logs, and traces.

<p align="center">
<img alt="Explore UI" style="border-radius: 1%;" src=grafana-profiles.gif>
</p>
[If you want to understand what profiling data looks like, try the profiling experience in play.grafana.com]()

[//TODO]: <> (link to live demo/play.grafana.com.)

Core features of Grafana Fire include:

- **Easy to install:** Using its monolithic mode, you can get Grafana Fire up and
  running with just one binary and no additional dependencies. On Kubernetes a single helm chart
  allows to deploy in different mode.
- **Horizontal scalability:**  You can run Grafana Fire
   across multiple machines, which makes it effortless for you to scale the database to handle the profiling volumes your workload generates.
- **High availability:** Grafana Fire replicates incoming profiles, ensuring that
  no data is lost in the event of machine failure. This means you can rollout without
  interrupting profiles ingestion and analysis.
- **Cheap, durable profile storage:** Grafana Fire uses object storage for long-term data storage,
  allowing it to take advantage of this ubiquitous, cost-effective, high-durability technology.
  It is compatible with multiple object store implementations, including AWS S3,
  Google Cloud Storage, Azure Blob Storage, OpenStack Swift, as well as any S3-compatible object storage.
- **Natively multi-tenant:** Grafana Fire's multi-tenant architecture enables you
  to isolate data and queries from independent teams or business units, making it
  possible for these groups to share the same database.

## Deploying Grafana Fire

For information about how to deploy Grafana Fire, refer to [Deploy Grafana Fire](https://grafana.com/docs/fire/latest/operators-guide/deploying-grafana-fire/).

## Getting started

If you’re new to Grafana Fire, read the [Getting started guide](https://grafana.com/docs/fire/latest/operators-guide/getting-started/).

Before deploying Grafana Fire in a production environment, read:

1. [An overview of Grafana Fire’s architecture](https://grafana.com/docs/fire/latest/operators-guide/architecture/)
1. [Configure Grafana Fire](https://grafana.com/docs/fire/latest/operators-guide/configuring/)
1. [Deploy in Kubernetes](https://grafana.com/docs/fire/latest/operators-guide/deploy-kubernetes/)

## Documentation

Refer to the following links to access Grafana Fire documentation:

- [Latest release](https://grafana.com/docs/fire/latest/)
- [Upcoming release](https://grafana.com/docs/fire/next/), at the tip of the main branch

## Contributing

To contribute to Grafana Fire, refer to [Contributing to Grafana Fire](https://github.com/grafana/fire/tree/main/docs/internal/contributing).

## Join the Grafana Fire discussion

If you have any questions or feedback regarding Grafana Fire, join the [Grafana Fire Discussion](https://github.com/grafana/fire/discussions). Alternatively, consider joining the monthly [Grafana Fire Community Call](TODO-doc-link).

Your feedback is always welcome, and you can also share it via the [`#fire` Slack channel](https://slack.grafana.com/).

## License

Grafana Fire is distributed under [AGPL-3.0-only](LICENSE).
