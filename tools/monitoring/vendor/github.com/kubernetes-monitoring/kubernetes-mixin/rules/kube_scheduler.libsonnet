{
  _config+:: {
    kubeSchedulerSelector: 'job="kube-scheduler"',
    podLabel: 'pod',
  },

  prometheusRules+:: {
    groups+: [
      {
        name: 'kube-scheduler.rules',
        rules: [
          {
            record: 'cluster_quantile:%s:histogram_quantile' % metric,
            expr: |||
              histogram_quantile(%(quantile)s, sum(rate(%(metric)s_bucket{%(kubeSchedulerSelector)s}[5m])) without(instance, %(podLabel)s))
            ||| % ({ quantile: quantile, metric: metric } + $._config),
            labels: {
              quantile: quantile,
            },
          }
          for quantile in ['0.99', '0.9', '0.5']
          for metric in [
            'scheduler_e2e_scheduling_duration_seconds',
            'scheduler_scheduling_algorithm_duration_seconds',
            'scheduler_binding_duration_seconds',
          ]
        ],
      },
    ],
  },
}
