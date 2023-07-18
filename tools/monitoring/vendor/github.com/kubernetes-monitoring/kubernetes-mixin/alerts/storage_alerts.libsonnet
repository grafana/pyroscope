{
  _config+:: {
    kubeStateMetricsSelector: error 'must provide selector for kube-state-metrics',
    kubeletSelector: error 'must provide selector for kubelet',
    namespaceSelector: null,
    prefixedNamespaceSelector: if self.namespaceSelector != null then self.namespaceSelector + ',' else '',

    // We alert when a disk is expected to fill up in four days. Depending on
    // the data-set it might be useful to change the sampling-time for the
    // prediction
    volumeFullPredictionSampleTime: '6h',
  },

  prometheusAlerts+:: {
    groups+: [
      {
        name: 'kubernetes-storage',
        rules: [
          {
            alert: 'KubePersistentVolumeFillingUp',
            expr: |||
              (
                kubelet_volume_stats_available_bytes{%(prefixedNamespaceSelector)s%(kubeletSelector)s}
                  /
                kubelet_volume_stats_capacity_bytes{%(prefixedNamespaceSelector)s%(kubeletSelector)s}
              ) < 0.03
              and
              kubelet_volume_stats_used_bytes{%(prefixedNamespaceSelector)s%(kubeletSelector)s} > 0
              unless on(namespace, persistentvolumeclaim)
              kube_persistentvolumeclaim_access_mode{%(prefixedNamespaceSelector)s access_mode="ReadOnlyMany"} == 1
              unless on(namespace, persistentvolumeclaim)
              kube_persistentvolumeclaim_labels{%(prefixedNamespaceSelector)s%(pvExcludedSelector)s} == 1
            ||| % $._config,
            'for': '1m',
            labels: {
              severity: 'critical',
            },
            annotations: {
              description: 'The PersistentVolume claimed by {{ $labels.persistentvolumeclaim }} in Namespace {{ $labels.namespace }} is only {{ $value | humanizePercentage }} free.',
              summary: 'PersistentVolume is filling up.',
            },
          },
          {
            alert: 'KubePersistentVolumeFillingUp',
            expr: |||
              (
                kubelet_volume_stats_available_bytes{%(prefixedNamespaceSelector)s%(kubeletSelector)s}
                  /
                kubelet_volume_stats_capacity_bytes{%(prefixedNamespaceSelector)s%(kubeletSelector)s}
              ) < 0.15
              and
              kubelet_volume_stats_used_bytes{%(prefixedNamespaceSelector)s%(kubeletSelector)s} > 0
              and
              predict_linear(kubelet_volume_stats_available_bytes{%(prefixedNamespaceSelector)s%(kubeletSelector)s}[%(volumeFullPredictionSampleTime)s], 4 * 24 * 3600) < 0
              unless on(namespace, persistentvolumeclaim)
              kube_persistentvolumeclaim_access_mode{%(prefixedNamespaceSelector)s access_mode="ReadOnlyMany"} == 1
              unless on(namespace, persistentvolumeclaim)
              kube_persistentvolumeclaim_labels{%(prefixedNamespaceSelector)s%(pvExcludedSelector)s} == 1
            ||| % $._config,
            'for': '1h',
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: 'Based on recent sampling, the PersistentVolume claimed by {{ $labels.persistentvolumeclaim }} in Namespace {{ $labels.namespace }} is expected to fill up within four days. Currently {{ $value | humanizePercentage }} is available.',
              summary: 'PersistentVolume is filling up.',
            },
          },
          {
            alert: 'KubePersistentVolumeInodesFillingUp',
            expr: |||
              (
                kubelet_volume_stats_inodes_free{%(prefixedNamespaceSelector)s%(kubeletSelector)s}
                  /
                kubelet_volume_stats_inodes{%(prefixedNamespaceSelector)s%(kubeletSelector)s}
              ) < 0.03
              and
              kubelet_volume_stats_inodes_used{%(prefixedNamespaceSelector)s%(kubeletSelector)s} > 0
              unless on(namespace, persistentvolumeclaim)
              kube_persistentvolumeclaim_access_mode{%(prefixedNamespaceSelector)s access_mode="ReadOnlyMany"} == 1
              unless on(namespace, persistentvolumeclaim)
              kube_persistentvolumeclaim_labels{%(prefixedNamespaceSelector)s%(pvExcludedSelector)s} == 1
            ||| % $._config,
            'for': '1m',
            labels: {
              severity: 'critical',
            },
            annotations: {
              description: 'The PersistentVolume claimed by {{ $labels.persistentvolumeclaim }} in Namespace {{ $labels.namespace }} only has {{ $value | humanizePercentage }} free inodes.',
              summary: 'PersistentVolumeInodes are filling up.',
            },
          },
          {
            alert: 'KubePersistentVolumeInodesFillingUp',
            expr: |||
              (
                kubelet_volume_stats_inodes_free{%(prefixedNamespaceSelector)s%(kubeletSelector)s}
                  /
                kubelet_volume_stats_inodes{%(prefixedNamespaceSelector)s%(kubeletSelector)s}
              ) < 0.15
              and
              kubelet_volume_stats_inodes_used{%(prefixedNamespaceSelector)s%(kubeletSelector)s} > 0
              and
              predict_linear(kubelet_volume_stats_inodes_free{%(prefixedNamespaceSelector)s%(kubeletSelector)s}[%(volumeFullPredictionSampleTime)s], 4 * 24 * 3600) < 0
              unless on(namespace, persistentvolumeclaim)
              kube_persistentvolumeclaim_access_mode{%(prefixedNamespaceSelector)s access_mode="ReadOnlyMany"} == 1
              unless on(namespace, persistentvolumeclaim)
              kube_persistentvolumeclaim_labels{%(prefixedNamespaceSelector)s%(pvExcludedSelector)s} == 1
            ||| % $._config,
            'for': '1h',
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: 'Based on recent sampling, the PersistentVolume claimed by {{ $labels.persistentvolumeclaim }} in Namespace {{ $labels.namespace }} is expected to run out of inodes within four days. Currently {{ $value | humanizePercentage }} of its inodes are free.',
              summary: 'PersistentVolumeInodes are filling up.',
            },
          },
          {
            alert: 'KubePersistentVolumeErrors',
            expr: |||
              kube_persistentvolume_status_phase{phase=~"Failed|Pending",%(prefixedNamespaceSelector)s%(kubeStateMetricsSelector)s} > 0
            ||| % $._config,
            'for': '5m',
            labels: {
              severity: 'critical',
            },
            annotations: {
              description: 'The persistent volume {{ $labels.persistentvolume }} has status {{ $labels.phase }}.',
              summary: 'PersistentVolume is having issues with provisioning.',
            },
          },
        ],
      },
    ],
  },
}
