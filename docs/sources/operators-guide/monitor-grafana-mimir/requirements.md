---
aliases:
  - ../visualizing-profiles/requirements/
  - /docs/fire/latest/operators-guide/monitoring-grafana-fire/requirements/
description: Requirements for installing Grafana Fire dashboards and alerts.
menuTitle: About dashboards and alerts requirements
title: About Grafana Fire dashboards and alerts requirements
weight: 10
---

# About Grafana Fire dashboards and alerts requirements

Grafana Fire dashboards and alerts require certain labels to exist on profiles scraped from Grafana Fire.

The `fire-distributed` Helm chart provides metamonitoring support, which takes care of these labels.
For more information about Helm chart metamonitoring, refer to [Collect profiles and logs via the Helm chart]({{< relref "collecting-profiles-and-logs.md#collect-profiles-and-logs-via-the-helm-chart" >}}).
If you are using Helm chart metamonitoring, go to [Installing Grafana Fire dashboards and alerts]({{< relref "installing-dashboards-and-alerts.md" >}}).

If you are not, then continue reading.
Your Prometheus or Grafana Agent must be configured to add these labels in order for the dashboards and alerts to function.
The following table shows the required label names and whether they can be customized when [compiling dashboards or alerts from sources]({{< relref "installing-dashboards-and-alerts.md" >}}).

| Label name  | Configurable? | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| :---------- | :------------ | :----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `cluster`   | Yes           | The Kubernetes cluster or datacenter where the Fire cluster is running. You can configure the cluster label via the `per_cluster_label` field in the mixin configuration.                                                                                                                                                                                                                                                                                                                             |
| `namespace` | No            | The Kubernetes namespace where the Fire cluster is running.                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| `job`       | Partially     | The Kubernetes namespace and Fire component in the format `<namespace>/<component>`. When running in monolithic mode, the `<component>` should be `fire`. When running in microservices mode, the `<component>` should be the name of the specific Fire component (singular), like `distributor`, `ingester` or `store-gateway`. You cannot configure the label name. But you can configure the regular expressions used to match components with the `job_names` field in the mixin configuration. |
| `pod`       | Yes           | The unique identifier of a Fire replica (eg. Pod ID when running on Kubernetes). You can configure the instance label via the `per_instance_label` field in the mixin configuration.                                                                                                                                                                                                                                                                                                                  |
| `instance`  | Yes           | The unique identifier of the node or machine where the Fire replica is running (eg. the node when running on Kubernetes). You can configure the node label via the `per_node_label` field in the mixin configuration.                                                                                                                                                                                                                                                                                 |

For rules and alerts to function properly, you must configure your Prometheus or Grafana Agent to scrape profiles from Grafana Fire at an interval of `15s` or shorter.

## Deployment type

By default, Grafana Fire dashboards assume Fire is deployed in containers orchestrated by Kubernetes.
If you're running Fire on baremental, you should set the configuration field `deployment_type: 'baremetal'` and [re-compile the dashboards]({{< relref "installing-dashboards-and-alerts.md" >}}).

## Job selection

A metric could be exposed by multiple Grafana Fire components, or even different applications running in the same namespace.
To provide accurate dashboards and alerts, we use the `job` label to select a metric from specific components.
A `job` is a combination of namespace and component, for example `<namespace>/ingester`.

Pre-compiled dashboards and alerts are shipped with a default configuration.
If you compile dashboards and alerts from source, you have the option to customize the regular expression used to select each Fire component through the `job_names` field in the mixin config.

### Default `job` selection in monolithic mode

When running Grafana Fire in monolithic mode and using the pre-compiled dashboards and alerts, the `job` label should be set to `<namespace>/fire`.

### Default `job` selection in microservices mode

When running Grafana Fire in microservices mode and using the pre-compiled dashboards and alerts, the `job` label should be set according to the following table.

| Fire service   | Expected `job` label          |
| :-------------- | :---------------------------- |
| Distributor     | `<namespace>/distributor`     |
| Ingester        | `<namespace>/ingester`        |
| Querier         | `<namespace>/querier`         |
| Ruler           | `<namespace>/ruler`           |
| Query-frontend  | `<namespace>/query-frontend`  |
| Query-scheduler | `<namespace>/query-scheduler` |
| Store-gateway   | `<namespace>/store-gateway`   |
| Compactor       | `<namespace>/compactor`       |

## Additional resources profiles

The Grafana Fire dashboards displaying CPU, memory, disk, and network resources utilization require Prometheus profiles scraped from the following endpoints:

- [cAdvisor](https://github.com/google/cadvisor)
- [kubelet](https://kubernetes.io/docs/concepts/cluster-administration/system-profiles/)
- [node_exporter](https://github.com/prometheus/node_exporter)
- [kube-state-profiles](https://github.com/kubernetes/kube-state-profiles) exporter

For more information about the kubelet profiles and cAdvisor profiles exported by the kubelet, refer to [Profiles For Kubernetes System Components](https://kubernetes.io/docs/concepts/cluster-administration/system-profiles/).

Profiles from kubelet, kube-state-profiles, and cAdvisor must all have a `cluster` label with the same value as in the
Fire profiles.

Profiles from node_exporter must all have an `instance` label on them that has the same value as the `instance` label on Fire profiles.

## Log labels

The **Slow queries** dashboard uses a Loki data source with the logs from Grafana Fire to visualize slow queries. The query-frontend component logs slow queries based on how you configured the `-query-frontend.log-queries-longer-than` parameter.
These logs need to have specific labels in order for the dashboard to work.

| Label name  | Configurable? | Description                                                                                                                                                                |
| :---------- | :------------ | :------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `cluster`   | Yes           | The Kubernetes cluster or datacenter where the Fire cluster is running. You can configure the cluster label via the `per_cluster_label` field in the mixin configuration. |
| `namespace` | No            | The Kubernetes namespace where the Fire cluster is running.                                                                                                               |
| `name`      | No            | Name of the component. For example, `query-frontend`.                                                                                                                      |
