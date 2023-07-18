{
  prometheusRules+:: {
    groups+: [
      {
        name: 'windows.node.rules',
        rules: [
          {
            // This rule gives the number of windows nodes
            record: 'node:windows_node:sum',
            expr: |||
              count (
                windows_system_system_up_time{%(windowsExporterSelector)s}
              )
            ||| % $._config,
          },
          {
            // This rule gives the number of CPUs per node.
            record: 'node:windows_node_num_cpu:sum',
            expr: |||
              count by (instance) (sum by (instance, core) (
                windows_cpu_time_total{%(windowsExporterSelector)s}
              ))
            ||| % $._config,
          },
          {
            // CPU utilisation is % CPU is not idle.
            record: ':windows_node_cpu_utilisation:avg1m',
            expr: |||
              1 - avg(rate(windows_cpu_time_total{%(windowsExporterSelector)s,mode="idle"}[1m]))
            ||| % $._config,
          },
          {
            // CPU utilisation is % CPU is not idle.
            record: 'node:windows_node_cpu_utilisation:avg1m',
            expr: |||
              1 - avg by (instance) (
                rate(windows_cpu_time_total{%(windowsExporterSelector)s,mode="idle"}[1m])
              )
            ||| % $._config,
          },
          {
            record: ':windows_node_memory_utilisation:',
            expr: |||
              1 -
              sum(windows_memory_available_bytes{%(windowsExporterSelector)s})
              /
              sum(windows_os_visible_memory_bytes{%(windowsExporterSelector)s})
            ||| % $._config,
          },
          // Add separate rules for Free & Total, so we can aggregate across clusters
          // in dashboards.
          {
            record: ':windows_node_memory_MemFreeCached_bytes:sum',
            expr: |||
              sum(windows_memory_available_bytes{%(windowsExporterSelector)s} + windows_memory_cache_bytes{%(windowsExporterSelector)s})
            ||| % $._config,
          },
          {
            record: 'node:windows_node_memory_totalCached_bytes:sum',
            expr: |||
              (windows_memory_cache_bytes{%(windowsExporterSelector)s} + windows_memory_modified_page_list_bytes{%(windowsExporterSelector)s} + windows_memory_standby_cache_core_bytes{%(windowsExporterSelector)s} + windows_memory_standby_cache_normal_priority_bytes{%(windowsExporterSelector)s} + windows_memory_standby_cache_reserve_bytes{%(windowsExporterSelector)s})
            ||| % $._config,
          },
          {
            record: ':windows_node_memory_MemTotal_bytes:sum',
            expr: |||
              sum(windows_os_visible_memory_bytes{%(windowsExporterSelector)s})
            ||| % $._config,
          },
          {
            // Available memory per node
            // SINCE 2018-02-08
            record: 'node:windows_node_memory_bytes_available:sum',
            expr: |||
              sum by (instance) (
                (windows_memory_available_bytes{%(windowsExporterSelector)s})
              )
            ||| % $._config,
          },
          {
            // Total memory per node
            record: 'node:windows_node_memory_bytes_total:sum',
            expr: |||
              sum by (instance) (
                windows_os_visible_memory_bytes{%(windowsExporterSelector)s}
              )
            ||| % $._config,
          },
          {
            // Memory utilisation per node, normalized by per-node memory
            record: 'node:windows_node_memory_utilisation:ratio',
            expr: |||
              (node:windows_node_memory_bytes_total:sum - node:windows_node_memory_bytes_available:sum)
              /
              scalar(sum(node:windows_node_memory_bytes_total:sum))
            |||,
          },
          {
            record: 'node:windows_node_memory_utilisation:',
            expr: |||
              1 - (node:windows_node_memory_bytes_available:sum / node:windows_node_memory_bytes_total:sum)
            ||| % $._config,
          },
          {
            record: 'node:windows_node_memory_swap_io_pages:irate',
            expr: |||
              irate(windows_memory_swap_page_operations_total{%(windowsExporterSelector)s}[5m])
            ||| % $._config,
          },
          {
            // Disk utilisation (ms spent, by rate() it's bound by 1 second)
            record: ':windows_node_disk_utilisation:avg_irate',
            expr: |||
              avg(irate(windows_logical_disk_read_seconds_total{%(windowsExporterSelector)s}[1m]) +
                  irate(windows_logical_disk_write_seconds_total{%(windowsExporterSelector)s}[1m])
                )
            ||| % $._config,
          },
          {
            // Disk utilisation (ms spent, by rate() it's bound by 1 second)
            record: 'node:windows_node_disk_utilisation:avg_irate',
            expr: |||
              avg by (instance) (
                (irate(windows_logical_disk_read_seconds_total{%(windowsExporterSelector)s}[1m]) +
                 irate(windows_logical_disk_write_seconds_total{%(windowsExporterSelector)s}[1m]))
              )
            ||| % $._config,
          },
          {
            record: 'node:windows_node_filesystem_usage:',
            expr: |||
              max by (instance,volume)(
                (windows_logical_disk_size_bytes{%(windowsExporterSelector)s}
              - windows_logical_disk_free_bytes{%(windowsExporterSelector)s})
              / windows_logical_disk_size_bytes{%(windowsExporterSelector)s}
              )
            ||| % $._config,
          },
          {
            record: 'node:windows_node_filesystem_avail:',
            expr: |||
              max by (instance, volume) (windows_logical_disk_free_bytes{%(windowsExporterSelector)s} / windows_logical_disk_size_bytes{%(windowsExporterSelector)s})
            ||| % $._config,
          },
          {
            record: ':windows_node_net_utilisation:sum_irate',
            expr: |||
              sum(irate(windows_net_bytes_total{%(windowsExporterSelector)s}[1m]))
            ||| % $._config,
          },
          {
            record: 'node:windows_node_net_utilisation:sum_irate',
            expr: |||
              sum by (instance) (
                (irate(windows_net_bytes_total{%(windowsExporterSelector)s}[1m]))
              )
            ||| % $._config,
          },
          {
            record: ':windows_node_net_saturation:sum_irate',
            expr: |||
              sum(irate(windows_net_packets_received_discarded_total{%(windowsExporterSelector)s}[1m])) +
              sum(irate(windows_net_packets_outbound_discarded_total{%(windowsExporterSelector)s}[1m]))
            ||| % $._config,
          },
          {
            record: 'node:windows_node_net_saturation:sum_irate',
            expr: |||
              sum by (instance) (
                (irate(windows_net_packets_received_discarded_total{%(windowsExporterSelector)s}[1m]) +
                irate(windows_net_packets_outbound_discarded_total{%(windowsExporterSelector)s}[1m]))
              )
            ||| % $._config,
          },
        ],
      },
      {
        name: 'windows.pod.rules',
        rules: [
          {
            record: 'windows_pod_container_available',
            expr: |||
              windows_container_available{%(windowsExporterSelector)s} * on(container_id) group_left(container, pod, namespace) max(kube_pod_container_info{%(kubeStateMetricsSelector)s}) by(container, container_id, pod, namespace)
            ||| % $._config,
          },
          {
            record: 'windows_container_total_runtime',
            expr: |||
              windows_container_cpu_usage_seconds_total{%(windowsExporterSelector)s} * on(container_id) group_left(container, pod, namespace) max(kube_pod_container_info{%(kubeStateMetricsSelector)s}) by(container, container_id, pod, namespace)
            ||| % $._config,
          },
          {
            record: 'windows_container_memory_usage',
            expr: |||
              windows_container_memory_usage_commit_bytes{%(windowsExporterSelector)s} * on(container_id) group_left(container, pod, namespace) max(kube_pod_container_info{%(kubeStateMetricsSelector)s}) by(container, container_id, pod, namespace)
            ||| % $._config,
          },
          {
            record: 'windows_container_private_working_set_usage',
            expr: |||
              windows_container_memory_usage_private_working_set_bytes{%(windowsExporterSelector)s} * on(container_id) group_left(container, pod, namespace) max(kube_pod_container_info{%(kubeStateMetricsSelector)s}) by(container, container_id, pod, namespace)
            ||| % $._config,
          },
          {
            record: 'windows_container_network_received_bytes_total',
            expr: |||
              windows_container_network_receive_bytes_total{%(windowsExporterSelector)s} * on(container_id) group_left(container, pod, namespace) max(kube_pod_container_info{%(kubeStateMetricsSelector)s}) by(container, container_id, pod, namespace)
            ||| % $._config,
          },
          {
            record: 'windows_container_network_transmitted_bytes_total',
            expr: |||
              windows_container_network_transmit_bytes_total{%(windowsExporterSelector)s} * on(container_id) group_left(container, pod, namespace) max(kube_pod_container_info{%(kubeStateMetricsSelector)s}) by(container, container_id, pod, namespace)
            ||| % $._config,
          },
          {
            record: 'kube_pod_windows_container_resource_memory_request',
            expr: |||
              max by (namespace, pod, container) (
                kube_pod_container_resource_requests{resource="memory",%(kubeStateMetricsSelector)s}
              ) * on(container,pod,namespace) (windows_pod_container_available)
            ||| % $._config,
          },
          {
            record: 'kube_pod_windows_container_resource_memory_limit',
            expr: |||
              kube_pod_container_resource_limits{resource="memory",%(kubeStateMetricsSelector)s} * on(container,pod,namespace) (windows_pod_container_available)
            ||| % $._config,
          },
          {
            record: 'kube_pod_windows_container_resource_cpu_cores_request',
            expr: |||
              max by (namespace, pod, container) (
                kube_pod_container_resource_requests{resource="cpu",%(kubeStateMetricsSelector)s}
              ) * on(container,pod,namespace) (windows_pod_container_available)
            ||| % $._config,
          },
          {
            record: 'kube_pod_windows_container_resource_cpu_cores_limit',
            expr: |||
              kube_pod_container_resource_limits{resource="cpu",%(kubeStateMetricsSelector)s} * on(container,pod,namespace) (windows_pod_container_available)
            ||| % $._config,
          },
          {
            record: 'namespace_pod_container:windows_container_cpu_usage_seconds_total:sum_rate',
            expr: |||
              sum by (namespace, pod, container) (
                rate(windows_container_total_runtime{}[5m])
              )
            ||| % $._config,
          },
        ],
      },
    ],
  },
}
