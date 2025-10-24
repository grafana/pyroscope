{{/*
Config Map content for rules provisioning
*/}}
{{- define "pyroscope-monitoring.rules-configmap" -}}
{{/*
List of rules
*/}}
data:
  prometheus.yaml: |
    ---
    otlp:
      keep_identifying_resource_attributes: true
      # Recommended attributes to be promoted to labels.
      promote_resource_attributes:
        - service.instance.id
        - service.name
        - service.namespace
        - service.version
        - cloud.availability_zone
        - cloud.region
        - container.name
        - deployment.environment.name
        - k8s.cluster.name
        - k8s.container.name
        - k8s.cronjob.name
        - k8s.daemonset.name
        - k8s.deployment.name
        - k8s.job.name
        - k8s.namespace.name
        - k8s.pod.name
        - k8s.replicaset.name
        - k8s.statefulset.name

    storage:
      tsdb:
        # A 10min time window is enough because it can easily absorb retries and network delays.
        out_of_order_time_window: 10m
    rule_files:
      - "/prometheus-rules/*.rules.yaml"

  k8s-rules-pod-owner.rules.yaml: |
    groups:
    - name: k8s.rules.pod_owner
      rules:
        - expr: |
            max by (cluster, namespace, workload, pod) (
              label_replace(
                label_replace(
                kube_pod_owner{{ "{" }}{{ .Values.dashboards.kubeStateMetricsSelector }}, owner_kind="ReplicaSet"},
                  "replicaset", "$1", "owner_name", "(.*)"
                ) * on(replicaset, namespace) group_left(owner_name) topk by(replicaset, namespace) (
                  1, max by (replicaset, namespace, owner_name) (
                    kube_replicaset_owner{{ "{" }}{{ .Values.dashboards.kubeStateMetricsSelector }}}
                  )
                ),
                "workload", "$1", "owner_name", "(.*)"
              )
            )
          labels:
            workload_type: deployment
          record: namespace_workload_pod:kube_pod_owner:relabel
        - expr: |
            max by (cluster, namespace, workload, pod) (
              label_replace(
                kube_pod_owner{{ "{" }}{{ .Values.dashboards.kubeStateMetricsSelector }}, owner_kind="DaemonSet"},
                "workload", "$1", "owner_name", "(.*)"
              )
            )
          labels:
            workload_type: daemonset
          record: namespace_workload_pod:kube_pod_owner:relabel
        - expr: |
            max by (cluster, namespace, workload, pod) (
              label_replace(
                kube_pod_owner{{ "{" }}{{ .Values.dashboards.kubeStateMetricsSelector }}, owner_kind="StatefulSet"},
                "workload", "$1", "owner_name", "(.*)"
              )
            )
          labels:
            workload_type: statefulset
          record: namespace_workload_pod:kube_pod_owner:relabel
        - expr: |
            max by (cluster, namespace, workload, pod) (
              label_replace(
                kube_pod_owner{{ "{" }}{{ .Values.dashboards.kubeStateMetricsSelector }}, owner_kind="Job"},
                "workload", "$1", "owner_name", "(.*)"
              )
            )
          labels:
            workload_type: job
          record: namespace_workload_pod:kube_pod_owner:relabel
  k8s-rules-pod-container-cpu-usage-seconds-total.rules.yaml: |
    groups:
    - name: k8s.rules.container_cpu_usage_seconds_total
      rules:
        - expr: >
            sum by (cluster, namespace, pod, container) (
              rate(container_cpu_usage_seconds_total{{ "{" }}{{ .Values.dashboards.cadvisorSelector }}, image!=""}[5m])
            ) * on (cluster, namespace, pod) group_left(node) topk by (cluster,
            namespace, pod) (
              1, max by(cluster, namespace, pod, node) (kube_pod_info{node!=""})
            )
          record: node_namespace_pod_container:container_cpu_usage_seconds_total:sum_rate5m
        - expr: >
            sum by (cluster, namespace, pod, container) (
              irate(container_cpu_usage_seconds_total{{ "{" }}{{ .Values.dashboards.cadvisorSelector }}, image!=""}[5m])
            ) * on (cluster, namespace, pod) group_left(node) topk by (cluster,
            namespace, pod) (
              1, max by(cluster, namespace, pod, node) (kube_pod_info{node!=""})
            )
          record: node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate
{{- end }}

{{/*
Get hash across all rules
*/}}
{{- define "pyroscope-monitoring.rules-hash" -}}
{{- include "pyroscope-monitoring.rules-configmap" .  | sha256sum }}
{{- end }}
