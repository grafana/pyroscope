{
  _config+:: {
    cadvisorSelector: 'job="cadvisor"',
    kubeStateMetricsSelector: 'job="kube-state-metrics"',
  },

  prometheusRules+:: {
    groups+: [
      {
        name: 'k8s.rules',
        rules: [
          {
            // Reduces cardinality of this timeseries by #cores, which makes it
            // more useable in dashboards.  Also, allows us to do things like
            // quantile_over_time(...) which would otherwise not be possible.
            record: 'node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate',
            expr: |||
              sum by (%(clusterLabel)s, namespace, pod, container) (
                irate(container_cpu_usage_seconds_total{%(cadvisorSelector)s, image!=""}[5m])
              ) * on (%(clusterLabel)s, namespace, pod) group_left(node) topk by (%(clusterLabel)s, namespace, pod) (
                1, max by(%(clusterLabel)s, namespace, pod, node) (kube_pod_info{node!=""})
              )
            ||| % $._config,
          },
          {
            record: 'node_namespace_pod_container:container_memory_working_set_bytes',
            expr: |||
              container_memory_working_set_bytes{%(cadvisorSelector)s, image!=""}
              * on (namespace, pod) group_left(node) topk by(namespace, pod) (1,
                max by(namespace, pod, node) (kube_pod_info{node!=""})
              )
            ||| % $._config,
          },
          {
            record: 'node_namespace_pod_container:container_memory_rss',
            expr: |||
              container_memory_rss{%(cadvisorSelector)s, image!=""}
              * on (namespace, pod) group_left(node) topk by(namespace, pod) (1,
                max by(namespace, pod, node) (kube_pod_info{node!=""})
              )
            ||| % $._config,
          },
          {
            record: 'node_namespace_pod_container:container_memory_cache',
            expr: |||
              container_memory_cache{%(cadvisorSelector)s, image!=""}
              * on (namespace, pod) group_left(node) topk by(namespace, pod) (1,
                max by(namespace, pod, node) (kube_pod_info{node!=""})
              )
            ||| % $._config,
          },
          {
            record: 'node_namespace_pod_container:container_memory_swap',
            expr: |||
              container_memory_swap{%(cadvisorSelector)s, image!=""}
              * on (namespace, pod) group_left(node) topk by(namespace, pod) (1,
                max by(namespace, pod, node) (kube_pod_info{node!=""})
              )
            ||| % $._config,
          },
          {
            record: 'cluster:namespace:pod_memory:active:kube_pod_container_resource_requests',
            expr: |||
              kube_pod_container_resource_requests{resource="memory",%(kubeStateMetricsSelector)s}  * on (namespace, pod, %(clusterLabel)s)
              group_left() max by (namespace, pod, %(clusterLabel)s) (
                (kube_pod_status_phase{phase=~"Pending|Running"} == 1)
              )
            ||| % $._config,
          },
          {
            record: 'namespace_memory:kube_pod_container_resource_requests:sum',
            expr: |||
              sum by (namespace, %(clusterLabel)s) (
                  sum by (namespace, pod, %(clusterLabel)s) (
                      max by (namespace, pod, container, %(clusterLabel)s) (
                        kube_pod_container_resource_requests{resource="memory",%(kubeStateMetricsSelector)s}
                      ) * on(namespace, pod, %(clusterLabel)s) group_left() max by (namespace, pod, %(clusterLabel)s) (
                        kube_pod_status_phase{phase=~"Pending|Running"} == 1
                      )
                  )
              )
            ||| % $._config,
          },
          {
            record: 'cluster:namespace:pod_cpu:active:kube_pod_container_resource_requests',
            expr: |||
              kube_pod_container_resource_requests{resource="cpu",%(kubeStateMetricsSelector)s}  * on (namespace, pod, %(clusterLabel)s)
              group_left() max by (namespace, pod, %(clusterLabel)s) (
                (kube_pod_status_phase{phase=~"Pending|Running"} == 1)
              )
            ||| % $._config,
          },
          {
            record: 'namespace_cpu:kube_pod_container_resource_requests:sum',
            expr: |||
              sum by (namespace, %(clusterLabel)s) (
                  sum by (namespace, pod, %(clusterLabel)s) (
                      max by (namespace, pod, container, %(clusterLabel)s) (
                        kube_pod_container_resource_requests{resource="cpu",%(kubeStateMetricsSelector)s}
                      ) * on(namespace, pod, %(clusterLabel)s) group_left() max by (namespace, pod, %(clusterLabel)s) (
                        kube_pod_status_phase{phase=~"Pending|Running"} == 1
                      )
                  )
              )
            ||| % $._config,
          },
          {
            record: 'cluster:namespace:pod_memory:active:kube_pod_container_resource_limits',
            expr: |||
              kube_pod_container_resource_limits{resource="memory",%(kubeStateMetricsSelector)s}  * on (namespace, pod, %(clusterLabel)s)
              group_left() max by (namespace, pod, %(clusterLabel)s) (
                (kube_pod_status_phase{phase=~"Pending|Running"} == 1)
              )
            ||| % $._config,
          },
          {
            record: 'namespace_memory:kube_pod_container_resource_limits:sum',
            expr: |||
              sum by (namespace, %(clusterLabel)s) (
                  sum by (namespace, pod, %(clusterLabel)s) (
                      max by (namespace, pod, container, %(clusterLabel)s) (
                        kube_pod_container_resource_limits{resource="memory",%(kubeStateMetricsSelector)s}
                      ) * on(namespace, pod, %(clusterLabel)s) group_left() max by (namespace, pod, %(clusterLabel)s) (
                        kube_pod_status_phase{phase=~"Pending|Running"} == 1
                      )
                  )
              )
            ||| % $._config,
          },
          {
            record: 'cluster:namespace:pod_cpu:active:kube_pod_container_resource_limits',
            expr: |||
              kube_pod_container_resource_limits{resource="cpu",%(kubeStateMetricsSelector)s}  * on (namespace, pod, %(clusterLabel)s)
              group_left() max by (namespace, pod, %(clusterLabel)s) (
               (kube_pod_status_phase{phase=~"Pending|Running"} == 1)
               )
            ||| % $._config,
          },
          {
            record: 'namespace_cpu:kube_pod_container_resource_limits:sum',
            expr: |||
              sum by (namespace, %(clusterLabel)s) (
                  sum by (namespace, pod, %(clusterLabel)s) (
                      max by (namespace, pod, container, %(clusterLabel)s) (
                        kube_pod_container_resource_limits{resource="cpu",%(kubeStateMetricsSelector)s}
                      ) * on(namespace, pod, %(clusterLabel)s) group_left() max by (namespace, pod, %(clusterLabel)s) (
                        kube_pod_status_phase{phase=~"Pending|Running"} == 1
                      )
                  )
              )
            ||| % $._config,
          },
          // workload aggregation for deployments
          {
            record: 'namespace_workload_pod:kube_pod_owner:relabel',
            expr: |||
              max by (%(clusterLabel)s, namespace, workload, pod) (
                label_replace(
                  label_replace(
                    kube_pod_owner{%(kubeStateMetricsSelector)s, owner_kind="ReplicaSet"},
                    "replicaset", "$1", "owner_name", "(.*)"
                  ) * on(replicaset, namespace) group_left(owner_name) topk by(replicaset, namespace) (
                    1, max by (replicaset, namespace, owner_name) (
                      kube_replicaset_owner{%(kubeStateMetricsSelector)s}
                    )
                  ),
                  "workload", "$1", "owner_name", "(.*)"
                )
              )
            ||| % $._config,
            labels: {
              workload_type: 'deployment',
            },
          },
          {
            record: 'namespace_workload_pod:kube_pod_owner:relabel',
            expr: |||
              max by (%(clusterLabel)s, namespace, workload, pod) (
                label_replace(
                  kube_pod_owner{%(kubeStateMetricsSelector)s, owner_kind="DaemonSet"},
                  "workload", "$1", "owner_name", "(.*)"
                )
              )
            ||| % $._config,
            labels: {
              workload_type: 'daemonset',
            },
          },
          {
            record: 'namespace_workload_pod:kube_pod_owner:relabel',
            expr: |||
              max by (%(clusterLabel)s, namespace, workload, pod) (
                label_replace(
                  kube_pod_owner{%(kubeStateMetricsSelector)s, owner_kind="StatefulSet"},
                  "workload", "$1", "owner_name", "(.*)"
                )
              )
            ||| % $._config,
            labels: {
              workload_type: 'statefulset',
            },
          },
          {
            record: 'namespace_workload_pod:kube_pod_owner:relabel',
            expr: |||
              max by (%(clusterLabel)s, namespace, workload, pod) (
                label_replace(
                  kube_pod_owner{%(kubeStateMetricsSelector)s, owner_kind="Job"},
                  "workload", "$1", "owner_name", "(.*)"
                )
              )
            ||| % $._config,
            labels: {
              workload_type: 'job',
            },
          },
        ],
      },
    ],
  },
}
