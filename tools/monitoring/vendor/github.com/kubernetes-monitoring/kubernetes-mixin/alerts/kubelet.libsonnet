{
  _config+:: {
    kubeStateMetricsSelector: error 'must provide selector for kube-state-metrics',
    kubeletSelector: error 'must provide selector for kubelet',
    kubeNodeUnreachableIgnoreKeys: [
      'ToBeDeletedByClusterAutoscaler',
      'cloud.google.com/impending-node-termination',
      'aws-node-termination-handler/spot-itn',
    ],

    kubeletCertExpirationWarningSeconds: 7 * 24 * 3600,
    kubeletCertExpirationCriticalSeconds: 1 * 24 * 3600,
  },

  prometheusAlerts+:: {
    groups+: [
      {
        name: 'kubernetes-system-kubelet',
        rules: [
          {
            expr: |||
              kube_node_status_condition{%(kubeStateMetricsSelector)s,condition="Ready",status="true"} == 0
            ||| % $._config,
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: '{{ $labels.node }} has been unready for more than 15 minutes.',
              summary: 'Node is not ready.',
            },
            'for': '15m',
            alert: 'KubeNodeNotReady',
          },
          {
            expr: |||
              (kube_node_spec_taint{%(kubeStateMetricsSelector)s,key="node.kubernetes.io/unreachable",effect="NoSchedule"} unless ignoring(key,value) kube_node_spec_taint{%(kubeStateMetricsSelector)s,key=~"%(kubeNodeUnreachableIgnoreKeys)s"}) == 1
            ||| % $._config {
              kubeNodeUnreachableIgnoreKeys: std.join('|', super.kubeNodeUnreachableIgnoreKeys),
            },
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: '{{ $labels.node }} is unreachable and some workloads may be rescheduled.',
              summary: 'Node is unreachable.',
            },
            'for': '15m',
            alert: 'KubeNodeUnreachable',
          },
          {
            alert: 'KubeletTooManyPods',
            // Some node has a capacity of 1 like AWS's Fargate and only exists while a pod is running on it.
            // We have to ignore this special node in the KubeletTooManyPods alert.
            expr: |||
              count by(%(clusterLabel)s, node) (
                (kube_pod_status_phase{%(kubeStateMetricsSelector)s,phase="Running"} == 1) * on(instance,pod,namespace,%(clusterLabel)s) group_left(node) topk by(instance,pod,namespace,%(clusterLabel)s) (1, kube_pod_info{%(kubeStateMetricsSelector)s})
              )
              /
              max by(%(clusterLabel)s, node) (
                kube_node_status_capacity{%(kubeStateMetricsSelector)s,resource="pods"} != 1
              ) > 0.95
            ||| % $._config,
            'for': '15m',
            labels: {
              severity: 'info',
            },
            annotations: {
              description: "Kubelet '{{ $labels.node }}' is running at {{ $value | humanizePercentage }} of its Pod capacity.",
              summary: 'Kubelet is running at capacity.',
            },
          },
          {
            alert: 'KubeNodeReadinessFlapping',
            expr: |||
              sum(changes(kube_node_status_condition{status="true",condition="Ready"}[15m])) by (%(clusterLabel)s, node) > 2
            ||| % $._config,
            'for': '15m',
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: 'The readiness status of node {{ $labels.node }} has changed {{ $value }} times in the last 15 minutes.',
              summary: 'Node readiness status is flapping.',
            },
          },
          {
            alert: 'KubeletPlegDurationHigh',
            expr: |||
              node_quantile:kubelet_pleg_relist_duration_seconds:histogram_quantile{quantile="0.99"} >= 10
            ||| % $._config,
            'for': '5m',
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: 'The Kubelet Pod Lifecycle Event Generator has a 99th percentile duration of {{ $value }} seconds on node {{ $labels.node }}.',
              summary: 'Kubelet Pod Lifecycle Event Generator is taking too long to relist.',
            },
          },
          {
            alert: 'KubeletPodStartUpLatencyHigh',
            expr: |||
              histogram_quantile(0.99, sum(rate(kubelet_pod_worker_duration_seconds_bucket{%(kubeletSelector)s}[5m])) by (%(clusterLabel)s, instance, le)) * on(%(clusterLabel)s, instance) group_left(node) kubelet_node_name{%(kubeletSelector)s} > 60
            ||| % $._config,
            'for': '15m',
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: 'Kubelet Pod startup 99th percentile latency is {{ $value }} seconds on node {{ $labels.node }}.',
              summary: 'Kubelet Pod startup latency is too high.',
            },
          },
          {
            alert: 'KubeletClientCertificateExpiration',
            expr: |||
              kubelet_certificate_manager_client_ttl_seconds < %(kubeletCertExpirationWarningSeconds)s
            ||| % $._config,
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: 'Client certificate for Kubelet on node {{ $labels.node }} expires in {{ $value | humanizeDuration }}.',
              summary: 'Kubelet client certificate is about to expire.',
            },
          },
          {
            alert: 'KubeletClientCertificateExpiration',
            expr: |||
              kubelet_certificate_manager_client_ttl_seconds < %(kubeletCertExpirationCriticalSeconds)s
            ||| % $._config,
            labels: {
              severity: 'critical',
            },
            annotations: {
              description: 'Client certificate for Kubelet on node {{ $labels.node }} expires in {{ $value | humanizeDuration }}.',
              summary: 'Kubelet client certificate is about to expire.',
            },
          },
          {
            alert: 'KubeletServerCertificateExpiration',
            expr: |||
              kubelet_certificate_manager_server_ttl_seconds < %(kubeletCertExpirationWarningSeconds)s
            ||| % $._config,
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: 'Server certificate for Kubelet on node {{ $labels.node }} expires in {{ $value | humanizeDuration }}.',
              summary: 'Kubelet server certificate is about to expire.',
            },
          },
          {
            alert: 'KubeletServerCertificateExpiration',
            expr: |||
              kubelet_certificate_manager_server_ttl_seconds < %(kubeletCertExpirationCriticalSeconds)s
            ||| % $._config,
            labels: {
              severity: 'critical',
            },
            annotations: {
              description: 'Server certificate for Kubelet on node {{ $labels.node }} expires in {{ $value | humanizeDuration }}.',
              summary: 'Kubelet server certificate is about to expire.',
            },
          },
          {
            alert: 'KubeletClientCertificateRenewalErrors',
            expr: |||
              increase(kubelet_certificate_manager_client_expiration_renew_errors[5m]) > 0
            ||| % $._config,
            labels: {
              severity: 'warning',
            },
            'for': '15m',
            annotations: {
              description: 'Kubelet on node {{ $labels.node }} has failed to renew its client certificate ({{ $value | humanize }} errors in the last 5 minutes).',
              summary: 'Kubelet has failed to renew its client certificate.',
            },
          },
          {
            alert: 'KubeletServerCertificateRenewalErrors',
            expr: |||
              increase(kubelet_server_expiration_renew_errors[5m]) > 0
            ||| % $._config,
            labels: {
              severity: 'warning',
            },
            'for': '15m',
            annotations: {
              description: 'Kubelet on node {{ $labels.node }} has failed to renew its server certificate ({{ $value | humanize }} errors in the last 5 minutes).',
              summary: 'Kubelet has failed to renew its server certificate.',
            },
          },
          (import '../lib/absent_alert.libsonnet') {
            componentName:: 'Kubelet',
            selector:: $._config.kubeletSelector,
          },
        ],
      },
    ],
  },
}
