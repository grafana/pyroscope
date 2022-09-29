---
aliases:
  - /docs/fire/latest/operators-guide/monitoring-grafana-fire/collecting-profiles-and-logs/
description: Learn how to collect profiles and logs from Grafana Fire itself
menuTitle: Collecting profiles and logs
title: Collecting profiles and logs from Grafana Fire
weight: 60
---

# Collecting profiles and logs from Grafana Fire

You can collect logs and profiles from a Fire or GEM cluster. To set up dashboards and alerts,
see [Installing Grafana Fire dashboards and alerts]({{< relref "installing-dashboards-and-alerts.md" >}})
or [Grafana Cloud: Self-hosted Grafana Fire integration](https://grafana.com/docs/grafana-cloud/integrations/integrations/integration-fire/)
.

It is easier and best to monitor a cluster if it was installed via
the [Grafana Fire Helm chart](https://github.com/grafana/fire/tree/main/operations/helm/charts/fire-distributed). It
is also possible to use this integration if Fire was deployed another way. For more information,
see [Collect profiles and logs without the Helm chart](#collect-profiles-and-logs-without-the-helm-chart).

## Collect profiles and logs from the Helm chart

To set up the collection of profiles and logs, follow the steps that are based on the version of the Helm chart that you
deployed:

- For a stable release:
  - \>= 3.x.x:
    See [Collect profiles and logs via the Helm chart](#collect-profiles-and-logs-via-the-helm-chart)
  - \< 3.x.x: See [Collect profiles and logs via Grafana Agent](#collect-profiles-and-logs-via-grafana-agent)
- For non-Helm installations or installations of the deprecated enterprise-profiles Helm chart, see [Collect profiles and logs without the helm chart](#collect-profiles-and-logs-without-the-helm-chart).

### Collect profiles and logs via the Helm chart

Starting from version `3.0.0`, the Helm chart sends profiles to a Prometheus-compatible server and sends logs to a Loki
cluster. The chart can also scrape additional profiles from kube-state-profiles, kubelet, and cAdvisor.
The Helm chart does not collect node_exporter profiles. For more information
about node_exporter, see [Additional resources profiles]({{< relref "requirements.md#additional-resources-profiles" >}}).

This section guides you through the process for setting up profiles and logs collection via
the [Grafana Agent operator](https://grafana.com/docs/agent/latest/operator/). The Fire Helm chart can install and use
the Grafana Agent operator. Due to how Helm works, before the chart can use the operator, you need to manually install
the [Custom Resource Definitions (CRDs)](https://github.com/grafana/agent/tree/main/production/operator/crds) for the
Agent operator.

Using the Agent operator for profiles and logs collection is our recommended approach. However, if you prefer not to use the Agent operator or already have an existing Grafana Agent you'd like to use for profiles and logs collection, follow the instructions
for [collecting profiles and logs via Grafana Agent](#collect-profiles-and-logs-via-grafana-agent) instead.

#### Credentials

If Prometheus and Loki are running without authentication, then you scan skip this section.
Metamonitoring supports multiple ways of authentication for profiles and logs. If you are using a secret such as an API
key to authenticate with Prometheus or Loki, then you need to create a Kubernetes secret with that secret.

This is an example secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: metamonitoring-credentials
data:
  prometheus-api-key: FAKEACCESSKEY
  loki-api-key: FAKESECRETKEY
```

For information about how to create a Kubernetes secret, see
[Creating a Secret](https://kubernetes.io/docs/concepts/configuration/secret/#creating-a-secret).

#### Helm chart values

Finally, merge the following YAML configuration into your Helm values file, and replace the values for `url`, `username`, `passwordSecretName`
, and `passwordSecretKey` with the details of the Prometheus and Loki clusters, and the secret that you created. If your
Prometheus and Loki servers are running without authentication, then remove the `auth` blocks from the YAML below.

If you already have the Agent operator installed in your Kubernetes cluster, then set `installOperator: false`.

```yaml
metaMonitoring:
  serviceMonitor:
    enabled: true
  grafanaAgent:
    enabled: true
    installOperator: true

    logs:
      remote:
        url: "https://example.com/loki/api/v1/push"
        auth:
          username: "12345"
          passwordSecretName: "metamonitoring-credentials"
          passwordSecretKey: "prometheus-api-key"

    profiles:
      remote:
        url: "https://example.com/api/v1/push"
        auth:
          username: "54321"
          passwordSecretName: "metamonitoring-credentials"
          passwordSecretKey: "loki-api-key"

      scrapeK8s:
        enabled: true
        kubeStateProfiles:
          namespace: kube-system
          labelSelectors:
            app.kubernetes.io/name: kube-state-profiles
```

### Collect profiles and logs via Grafana Agent

Older versions of the Helm chart need to be manually instrumented. This means that you need to set up a Grafana Agent
that collects logs and profiles from Fire or GEM. To set up Grafana Agent,
see [Set up Grafana Agent](https://grafana.com/docs/agent/latest/set-up/). Once your Agent is deployed, use the [example Agent configuration](#example-agent-configuration) to configure the Agent to scrape Fire or GEM.

#### Caveats

Managing your own Agent comes with some caveats:

- You will have to keep the Agent configuration up to date manually as you update the Fire Helm chart. While we will
  try to keep this article up to date, we cannot guarantee that
  the [example Agent configuration](#example-agent-configuration) will always work.
- The static configuration makes some assumptions about the naming of the chart, such as that you have not overridden
  the `fullnameOverride` in the Helm chart.
- The static configuration cannot be selective in the PersistentVolumes profiles it collects from Kubelet, so it will
  scrape profiles for all PersistentVolumes.
- The static configuration hardcodes the value of the `cluster` label on all profiles and logs. This means that the
  configuration cannot account for multiple installations of the Helm chart.

If possible, upgrade the Fire Helm chart to version 3.0 or higher and use
the [built-in Grafana Agent operator](#collect-profiles-and-logs-via-the-helm-chart). Using the Agent operator allows the
chart to automatically configure the Agent, eliminating the aforementioned caveats.

#### Example Agent configuration

In the following example Grafana Agent configuration file for collecting logs and profiles, replace `url`, `password`, and `username` in
the `logs` and `profiles` blocks with the details of your Prometheus and Loki clusters.

```yaml
logs:
  configs:
    - clients:
        - basic_auth:
            password: xxx
            username: xxx
          url: https://example.com/loki/api/v1/push
      name: integrations
      positions:
        filename: /tmp/positions.yaml
      scrape_configs:
        - job_name: integrations/grafana-fire-logs
          kubernetes_sd_configs:
            - role: pod
          pipeline_stages:
            - cri: {}
          relabel_configs:
            - action: keep
              regex: fire-distributed-.*
              source_labels:
                - __meta_kubernetes_pod_label_helm_sh_chart
            - source_labels:
                - __meta_kubernetes_pod_node_name
              target_label: __host__
            - action: replace
              replacement: $1
              separator: /
              source_labels:
                - __meta_kubernetes_namespace
                - __meta_kubernetes_pod_container_name
              target_label: job
            - action: replace
              regex: ""
              replacement: k8s-cluster
              separator: ""
              source_labels:
                - cluster
              target_label: cluster
            - action: replace
              source_labels:
                - __meta_kubernetes_namespace
              target_label: namespace
            - action: replace
              source_labels:
                - __meta_kubernetes_pod_name
              target_label: pod
            - action: replace
              source_labels:
                - __meta_kubernetes_pod_container_name
              target_label: name
            - action: replace
              source_labels:
                - __meta_kubernetes_pod_container_name
              target_label: container
            - replacement: /var/log/pods/*$1/*.log
              separator: /
              source_labels:
                - __meta_kubernetes_pod_uid
                - __meta_kubernetes_pod_container_name
              target_label: __path__
      target_config:
        sync_period: 10s
profiles:
  configs:
    - name: integrations
      remote_write:
        - basic_auth:
            password: xxx
            username: xxx
          url: https://example.com/api/prom/push
      scrape_configs:
        - job_name: integrations/grafana-fire/kube-state-profiles
          kubernetes_sd_configs:
            - role: pod
          metric_relabel_configs:
            - action: keep
              regex: (.*-fire-)?alertmanager.*|(.*-fire-)?compactor.*|(.*-fire-)?distributor.*|(.*-fire-)?(gateway|cortex-gw|cortex-gw).*|(.*-fire-)?ingester.*|(.*-fire-)?querier.*|(.*-fire-)?query-frontend.*|(.*-fire-)?query-scheduler.*|(.*-fire-)?ruler.*|(.*-fire-)?store-gateway.*
              separator: ""
              source_labels:
                - deployment
                - statefulset
                - pod
          relabel_configs:
            - action: keep
              regex: kube-state-profiles
              source_labels:
                - __meta_kubernetes_pod_label_app_kubernetes_io_name
            - action: replace
              regex: ""
              replacement: k8s-cluster
              separator: ""
              source_labels:
                - cluster
              target_label: cluster
        - bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token
          job_name: integrations/grafana-fire/kubelet
          kubernetes_sd_configs:
            - role: node
          metric_relabel_configs:
            - action: keep
              regex: kubelet_volume_stats.*
              source_labels:
                - __name__
          relabel_configs:
            - replacement: kubernetes.default.svc.cluster.local:443
              target_label: __address__
            - regex: (.+)
              replacement: /api/v1/nodes/${1}/proxy/profiles
              source_labels:
                - __meta_kubernetes_node_name
              target_label: __profiles_path__
            - action: replace
              regex: ""
              replacement: k8s-cluster
              separator: ""
              source_labels:
                - cluster
              target_label: cluster
          scheme: https
          tls_config:
            ca_file: /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
            insecure_skip_verify: false
            server_name: kubernetes
        - bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token
          job_name: integrations/grafana-fire/cadvisor
          kubernetes_sd_configs:
            - role: node
          metric_relabel_configs:
            - action: keep
              regex: (.*-fire-)?alertmanager.*|(.*-fire-)?compactor.*|(.*-fire-)?distributor.*|(.*-fire-)?(gateway|cortex-gw|cortex-gw).*|(.*-fire-)?ingester.*|(.*-fire-)?querier.*|(.*-fire-)?query-frontend.*|(.*-fire-)?query-scheduler.*|(.*-fire-)?ruler.*|(.*-fire-)?store-gateway.*
              source_labels:
                - pod
          relabel_configs:
            - replacement: kubernetes.default.svc.cluster.local:443
              target_label: __address__
            - regex: (.+)
              replacement: /api/v1/nodes/${1}/proxy/profiles/cadvisor
              source_labels:
                - __meta_kubernetes_node_name
              target_label: __profiles_path__
            - action: replace
              regex: ""
              replacement: k8s-cluster
              separator: ""
              source_labels:
                - cluster
              target_label: cluster
          scheme: https
          tls_config:
            ca_file: /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
            insecure_skip_verify: false
            server_name: kubernetes
        - job_name: integrations/grafana-fire/profiles
          kubernetes_sd_configs:
            - role: pod
          relabel_configs:
            - action: keep
              regex: .*profiles
              source_labels:
                - __meta_kubernetes_pod_container_port_name
            - action: keep
              regex: fire-distributed-.*
              source_labels:
                - __meta_kubernetes_pod_label_helm_sh_chart
            - action: replace
              regex: ""
              replacement: k8s-cluster
              separator: ""
              source_labels:
                - cluster
              target_label: cluster
            - action: replace
              source_labels:
                - __meta_kubernetes_namespace
              target_label: namespace
            - action: replace
              source_labels:
                - __meta_kubernetes_pod_name
              target_label: pod
            - action: replace
              source_labels:
                - __meta_kubernetes_pod_container_name
              target_label: container
            - action: replace
              separator: ""
              source_labels:
                - __meta_kubernetes_pod_label_name
                - __meta_kubernetes_pod_label_app_kubernetes_io_component
              target_label: __tmp_component_name
            - action: replace
              separator: /
              source_labels:
                - __meta_kubernetes_namespace
                - __tmp_component_name
              target_label: job
            - action: replace
              source_labels:
                - __meta_kubernetes_pod_node_name
              target_label: instance
  global:
    scrape_interval: 15s
  wal_directory: /tmp/grafana-agent-wal
```

## Collect profiles and logs without the Helm chart

You can still use the dashboards and rules in the monitoring-mixin, even if Fire or GEM is not deployed via the Helm
chart or if you are using the deprecated enterprise-profiles Helm chart for GEM.
As a starting point, use the Agent configuration
from [Collect profiles and logs via Grafana Agent](#collect-profiles-and-logs-via-grafana-agent).
You might need to modify it. For
more information, see [dashboards and alerts requirements]({{< relref "requirements.md" >}}).

### Service discovery

The Agent configuration relies on Kubernetes service discovery and pod labels to constrain the collected profiles and
logs to ones that are strictly related to the Helm chart. If you are deploying Grafana Fire on something other than Kubernetes,
then replace the `kubernetes_sd_configs` block with a block from
the [Agent configuration](https://grafana.com/docs/agent/latest/configuration/) that can discover the Fire processes.
