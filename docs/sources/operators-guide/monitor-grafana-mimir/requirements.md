---
aliases:
  - ../visualizing-metrics/requirements/
  - /docs/mimir/latest/operators-guide/monitoring-grafana-mimir/requirements/
description: Requirements for installing Grafana Mimir dashboards and alerts.
menuTitle: About dashboards and alerts requirements
title: About Grafana Mimir dashboards and alerts requirements
weight: 10
---

# About Grafana Mimir dashboards and alerts requirements

Grafana Mimir dashboards and alerts require certain labels to exist on metrics scraped from Grafana Mimir.

The `mimir-distributed` Helm chart provides metamonitoring support, which takes care of these labels.
For more information about Helm chart metamonitoring, refer to [Collect metrics and logs via the Helm chart]({{< relref "collecting-metrics-and-logs.md#collect-metrics-and-logs-via-the-helm-chart" >}}).
If you are using Helm chart metamonitoring, go to [Installing Grafana Mimir dashboards and alerts]({{< relref "installing-dashboards-and-alerts.md" >}}).

If you are not, then continue reading.
Your Prometheus or Grafana Agent must be configured to add these labels in order for the dashboards and alerts to function.
The following table shows the required label names and whether they can be customized when [compiling dashboards or alerts from sources]({{< relref "installing-dashboards-and-alerts.md" >}}).

| Label name  | Configurable? | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| :---------- | :------------ | :----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `cluster`   | Yes           | The Kubernetes cluster or datacenter where the Mimir cluster is running. You can configure the cluster label via the `per_cluster_label` field in the mixin configuration.                                                                                                                                                                                                                                                                                                                             |
| `namespace` | No            | The Kubernetes namespace where the Mimir cluster is running.                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| `job`       | Partially     | The Kubernetes namespace and Mimir component in the format `<namespace>/<component>`. When running in monolithic mode, the `<component>` should be `mimir`. When running in microservices mode, the `<component>` should be the name of the specific Mimir component (singular), like `distributor`, `ingester` or `store-gateway`. You cannot configure the label name. But you can configure the regular expressions used to match components with the `job_names` field in the mixin configuration. |
| `pod`       | Yes           | The unique identifier of a Mimir replica (eg. Pod ID when running on Kubernetes). You can configure the instance label via the `per_instance_label` field in the mixin configuration.                                                                                                                                                                                                                                                                                                                  |
| `instance`  | Yes           | The unique identifier of the node or machine where the Mimir replica is running (eg. the node when running on Kubernetes). You can configure the node label via the `per_node_label` field in the mixin configuration.                                                                                                                                                                                                                                                                                 |

For rules and alerts to function properly, you must configure your Prometheus or Grafana Agent to scrape metrics from Grafana Mimir at an interval of `15s` or shorter.

## Deployment type

By default, Grafana Mimir dashboards assume Mimir is deployed in containers orchestrated by Kubernetes.
If you're running Mimir on baremental, you should set the configuration field `deployment_type: 'baremetal'` and [re-compile the dashboards]({{< relref "installing-dashboards-and-alerts.md" >}}).

## Job selection

A metric could be exposed by multiple Grafana Mimir components, or even different applications running in the same namespace.
To provide accurate dashboards and alerts, we use the `job` label to select a metric from specific components.
A `job` is a combination of namespace and component, for example `<namespace>/ingester`.

Pre-compiled dashboards and alerts are shipped with a default configuration.
If you compile dashboards and alerts from source, you have the option to customize the regular expression used to select each Mimir component through the `job_names` field in the mixin config.

### Default `job` selection in monolithic mode

When running Grafana Mimir in monolithic mode and using the pre-compiled dashboards and alerts, the `job` label should be set to `<namespace>/mimir`.

### Default `job` selection in microservices mode

When running Grafana Mimir in microservices mode and using the pre-compiled dashboards and alerts, the `job` label should be set according to the following table.

| Mimir service   | Expected `job` label          |
| :-------------- | :---------------------------- |
| Distributor     | `<namespace>/distributor`     |
| Ingester        | `<namespace>/ingester`        |
| Querier         | `<namespace>/querier`         |
| Ruler           | `<namespace>/ruler`           |
| Query-frontend  | `<namespace>/query-frontend`  |
| Query-scheduler | `<namespace>/query-scheduler` |
| Store-gateway   | `<namespace>/store-gateway`   |
| Compactor       | `<namespace>/compactor`       |

## Additional resources metrics

The Grafana Mimir dashboards displaying CPU, memory, disk, and network resources utilization require Prometheus metrics scraped from the following endpoints:

- [cAdvisor](https://github.com/google/cadvisor)
- [kubelet](https://kubernetes.io/docs/concepts/cluster-administration/system-metrics/)
- [node_exporter](https://github.com/prometheus/node_exporter)
- [kube-state-metrics](https://github.com/kubernetes/kube-state-metrics) exporter

For more information about the kubelet metrics and cAdvisor metrics exported by the kubelet, refer to [Metrics For Kubernetes System Components](https://kubernetes.io/docs/concepts/cluster-administration/system-metrics/).

Metrics from kubelet, kube-state-metrics, and cAdvisor must all have a `cluster` label with the same value as in the
Mimir metrics.

Metrics from node_exporter must all have an `instance` label on them that has the same value as the `instance` label on Mimir metrics.

## Log labels

The **Slow queries** dashboard uses a Loki data source with the logs from Grafana Mimir to visualize slow queries. The query-frontend component logs slow queries based on how you configured the `-query-frontend.log-queries-longer-than` parameter.
These logs need to have specific labels in order for the dashboard to work.

| Label name  | Configurable? | Description                                                                                                                                                                |
| :---------- | :------------ | :------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `cluster`   | Yes           | The Kubernetes cluster or datacenter where the Mimir cluster is running. You can configure the cluster label via the `per_cluster_label` field in the mixin configuration. |
| `namespace` | No            | The Kubernetes namespace where the Mimir cluster is running.                                                                                                               |
| `name`      | No            | Name of the component. For example, `query-frontend`.                                                                                                                      |
