{
  _config+:: {
    kubeStateMetricsSelector: error 'must provide selector for kube-state-metrics',
    kubeJobTimeoutDuration: error 'must provide value for kubeJobTimeoutDuration',
    namespaceSelector: null,
    prefixedNamespaceSelector: if self.namespaceSelector != null then self.namespaceSelector + ',' else '',
  },

  prometheusAlerts+:: {
    groups+: [
      {
        name: 'kubernetes-apps',
        rules: [
          {
            expr: |||
              max_over_time(kube_pod_container_status_waiting_reason{reason="CrashLoopBackOff", %(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}[5m]) >= 1
            ||| % $._config,
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: 'Pod {{ $labels.namespace }}/{{ $labels.pod }} ({{ $labels.container }}) is in waiting state (reason: "CrashLoopBackOff").',
              summary: 'Pod is crash looping.',
            },
            'for': '15m',
            alert: 'KubePodCrashLooping',
          },
          {
            // We wrap kube_pod_owner with the topk() aggregator to ensure that
            // every (namespace, pod, %(clusterLabel)s) tuple is unique even if the "owner_kind"
            // label exists for 2 values. This avoids "many-to-many matching
            // not allowed" errors when joining with kube_pod_status_phase.
            expr: |||
              sum by (namespace, pod, %(clusterLabel)s) (
                max by(namespace, pod, %(clusterLabel)s) (
                  kube_pod_status_phase{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s, phase=~"Pending|Unknown"}
                ) * on(namespace, pod, %(clusterLabel)s) group_left(owner_kind) topk by(namespace, pod, %(clusterLabel)s) (
                  1, max by(namespace, pod, owner_kind, %(clusterLabel)s) (kube_pod_owner{owner_kind!="Job"})
                )
              ) > 0
            ||| % $._config,
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: 'Pod {{ $labels.namespace }}/{{ $labels.pod }} has been in a non-ready state for longer than 15 minutes.',
              summary: 'Pod has been in a non-ready state for more than 15 minutes.',
            },
            'for': '15m',
            alert: 'KubePodNotReady',
          },
          {
            expr: |||
              kube_deployment_status_observed_generation{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}
                !=
              kube_deployment_metadata_generation{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}
            ||| % $._config,
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: 'Deployment generation for {{ $labels.namespace }}/{{ $labels.deployment }} does not match, this indicates that the Deployment has failed but has not been rolled back.',
              summary: 'Deployment generation mismatch due to possible roll-back',
            },
            'for': '15m',
            alert: 'KubeDeploymentGenerationMismatch',
          },
          {
            expr: |||
              (
                kube_deployment_spec_replicas{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}
                  >
                kube_deployment_status_replicas_available{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}
              ) and (
                changes(kube_deployment_status_replicas_updated{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}[10m])
                  ==
                0
              )
            ||| % $._config,
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: 'Deployment {{ $labels.namespace }}/{{ $labels.deployment }} has not matched the expected number of replicas for longer than 15 minutes.',
              summary: 'Deployment has not matched the expected number of replicas.',
            },
            'for': '15m',
            alert: 'KubeDeploymentReplicasMismatch',
          },
          {
            expr: |||
              (
                kube_statefulset_status_replicas_ready{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}
                  !=
                kube_statefulset_status_replicas{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}
              ) and (
                changes(kube_statefulset_status_replicas_updated{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}[10m])
                  ==
                0
              )
            ||| % $._config,
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: 'StatefulSet {{ $labels.namespace }}/{{ $labels.statefulset }} has not matched the expected number of replicas for longer than 15 minutes.',
              summary: 'Deployment has not matched the expected number of replicas.',
            },
            'for': '15m',
            alert: 'KubeStatefulSetReplicasMismatch',
          },
          {
            expr: |||
              kube_statefulset_status_observed_generation{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}
                !=
              kube_statefulset_metadata_generation{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}
            ||| % $._config,
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: 'StatefulSet generation for {{ $labels.namespace }}/{{ $labels.statefulset }} does not match, this indicates that the StatefulSet has failed but has not been rolled back.',
              summary: 'StatefulSet generation mismatch due to possible roll-back',
            },
            'for': '15m',
            alert: 'KubeStatefulSetGenerationMismatch',
          },
          {
            expr: |||
              (
                max without (revision) (
                  kube_statefulset_status_current_revision{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}
                    unless
                  kube_statefulset_status_update_revision{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}
                )
                  *
                (
                  kube_statefulset_replicas{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}
                    !=
                  kube_statefulset_status_replicas_updated{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}
                )
              )  and (
                changes(kube_statefulset_status_replicas_updated{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}[5m])
                  ==
                0
              )
            ||| % $._config,
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: 'StatefulSet {{ $labels.namespace }}/{{ $labels.statefulset }} update has not been rolled out.',
              summary: 'StatefulSet update has not been rolled out.',
            },
            'for': '15m',
            alert: 'KubeStatefulSetUpdateNotRolledOut',
          },
          {
            alert: 'KubeDaemonSetRolloutStuck',
            expr: |||
              (
                (
                  kube_daemonset_status_current_number_scheduled{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}
                   !=
                  kube_daemonset_status_desired_number_scheduled{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}
                ) or (
                  kube_daemonset_status_number_misscheduled{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}
                   !=
                  0
                ) or (
                  kube_daemonset_status_updated_number_scheduled{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}
                   !=
                  kube_daemonset_status_desired_number_scheduled{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}
                ) or (
                  kube_daemonset_status_number_available{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}
                   !=
                  kube_daemonset_status_desired_number_scheduled{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}
                )
              ) and (
                changes(kube_daemonset_status_updated_number_scheduled{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}[5m])
                  ==
                0
              )
            ||| % $._config,
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: 'DaemonSet {{ $labels.namespace }}/{{ $labels.daemonset }} has not finished or progressed for at least 15 minutes.',
              summary: 'DaemonSet rollout is stuck.',
            },
            'for': '15m',
          },
          {
            expr: |||
              sum by (namespace, pod, container, %(clusterLabel)s) (kube_pod_container_status_waiting_reason{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}) > 0
            ||| % $._config,
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: 'pod/{{ $labels.pod }} in namespace {{ $labels.namespace }} on container {{ $labels.container}} has been in waiting state for longer than 1 hour.',
              summary: 'Pod container waiting longer than 1 hour',
            },
            'for': '1h',
            alert: 'KubeContainerWaiting',
          },
          {
            alert: 'KubeDaemonSetNotScheduled',
            expr: |||
              kube_daemonset_status_desired_number_scheduled{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}
                -
              kube_daemonset_status_current_number_scheduled{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s} > 0
            ||| % $._config,
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: '{{ $value }} Pods of DaemonSet {{ $labels.namespace }}/{{ $labels.daemonset }} are not scheduled.',
              summary: 'DaemonSet pods are not scheduled.',
            },
            'for': '10m',
          },
          {
            alert: 'KubeDaemonSetMisScheduled',
            expr: |||
              kube_daemonset_status_number_misscheduled{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s} > 0
            ||| % $._config,
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: '{{ $value }} Pods of DaemonSet {{ $labels.namespace }}/{{ $labels.daemonset }} are running where they are not supposed to run.',
              summary: 'DaemonSet pods are misscheduled.',
            },
            'for': '15m',
          },
          {
            alert: 'KubeJobNotCompleted',
            expr: |||
              time() - max by(namespace, job_name, %(clusterLabel)s) (kube_job_status_start_time{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}
                and
              kube_job_status_active{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s} > 0) > %(kubeJobTimeoutDuration)s
            ||| % $._config,
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: 'Job {{ $labels.namespace }}/{{ $labels.job_name }} is taking more than {{ "%(kubeJobTimeoutDuration)s" | humanizeDuration }} to complete.' % $._config,
              summary: 'Job did not complete in time',
            },
          },
          {
            alert: 'KubeJobFailed',
            expr: |||
              kube_job_failed{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}  > 0
            ||| % $._config,
            'for': '15m',
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: 'Job {{ $labels.namespace }}/{{ $labels.job_name }} failed to complete. Removing failed job after investigation should clear this alert.',
              summary: 'Job failed to complete.',
            },
          },
          {
            expr: |||
              (kube_horizontalpodautoscaler_status_desired_replicas{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}
                !=
              kube_horizontalpodautoscaler_status_current_replicas{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s})
                and
              (kube_horizontalpodautoscaler_status_current_replicas{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}
                >
              kube_horizontalpodautoscaler_spec_min_replicas{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s})
                and
              (kube_horizontalpodautoscaler_status_current_replicas{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}
                <
              kube_horizontalpodautoscaler_spec_max_replicas{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s})
                and
              changes(kube_horizontalpodautoscaler_status_current_replicas{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}[15m]) == 0
            ||| % $._config,
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: 'HPA {{ $labels.namespace }}/{{ $labels.horizontalpodautoscaler  }} has not matched the desired number of replicas for longer than 15 minutes.',
              summary: 'HPA has not matched descired number of replicas.',
            },
            'for': '15m',
            alert: 'KubeHpaReplicasMismatch',
          },
          {
            expr: |||
              kube_horizontalpodautoscaler_status_current_replicas{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}
                ==
              kube_horizontalpodautoscaler_spec_max_replicas{%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s}
            ||| % $._config,
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: 'HPA {{ $labels.namespace }}/{{ $labels.horizontalpodautoscaler  }} has been running at max replicas for longer than 15 minutes.',
              summary: 'HPA is running at max replicas',
            },
            'for': '15m',
            alert: 'KubeHpaMaxedOut',
          },
        ],
      },
    ],
  },
}
