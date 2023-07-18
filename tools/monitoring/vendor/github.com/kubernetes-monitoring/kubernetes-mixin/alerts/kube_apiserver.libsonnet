local utils = import '../lib/utils.libsonnet';

{
  _config+:: {
    kubeApiserverSelector: error 'must provide selector for kube-apiserver',

    kubeAPILatencyWarningSeconds: 1,

    certExpirationWarningSeconds: 7 * 24 * 3600,
    certExpirationCriticalSeconds: 1 * 24 * 3600,
  },

  prometheusAlerts+:: {
    groups+: [
      {
        name: 'kube-apiserver-slos',
        rules: [
          {
            alert: 'KubeAPIErrorBudgetBurn',
            expr: |||
              sum(apiserver_request:burnrate%s) > (%.2f * %.5f)
              and
              sum(apiserver_request:burnrate%s) > (%.2f * %.5f)
            ||| % [
              w.long,
              w.factor,
              (1 - $._config.SLOs.apiserver.target),
              w.short,
              w.factor,
              (1 - $._config.SLOs.apiserver.target),
            ],
            labels: {
              severity: w.severity,
              short: '%(short)s' % w,
              long: '%(long)s' % w,
            },
            annotations: {
              description: 'The API server is burning too much error budget.',
              summary: 'The API server is burning too much error budget.',
            },
            'for': '%(for)s' % w,
          }
          for w in $._config.SLOs.apiserver.windows
        ],
      },
      {
        name: 'kubernetes-system-apiserver',
        rules: [
          {
            alert: 'KubeClientCertificateExpiration',
            expr: |||
              apiserver_client_certificate_expiration_seconds_count{%(kubeApiserverSelector)s} > 0 and on(job) histogram_quantile(0.01, sum by (job, le) (rate(apiserver_client_certificate_expiration_seconds_bucket{%(kubeApiserverSelector)s}[5m]))) < %(certExpirationWarningSeconds)s
            ||| % $._config,
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: 'A client certificate used to authenticate to kubernetes apiserver is expiring in less than %s.' % (utils.humanizeSeconds($._config.certExpirationWarningSeconds)),
              summary: 'Client certificate is about to expire.',
            },
          },
          {
            alert: 'KubeClientCertificateExpiration',
            expr: |||
              apiserver_client_certificate_expiration_seconds_count{%(kubeApiserverSelector)s} > 0 and on(job) histogram_quantile(0.01, sum by (job, le) (rate(apiserver_client_certificate_expiration_seconds_bucket{%(kubeApiserverSelector)s}[5m]))) < %(certExpirationCriticalSeconds)s
            ||| % $._config,
            labels: {
              severity: 'critical',
            },
            annotations: {
              description: 'A client certificate used to authenticate to kubernetes apiserver is expiring in less than %s.' % (utils.humanizeSeconds($._config.certExpirationCriticalSeconds)),
              summary: 'Client certificate is about to expire.',
            },
          },
          {
            alert: 'KubeAggregatedAPIErrors',
            expr: |||
              sum by(name, namespace, %(clusterLabel)s)(increase(aggregator_unavailable_apiservice_total[10m])) > 4
            ||| % $._config,
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: 'Kubernetes aggregated API {{ $labels.name }}/{{ $labels.namespace }} has reported errors. It has appeared unavailable {{ $value | humanize }} times averaged over the past 10m.',
              summary: 'Kubernetes aggregated API has reported errors.',
            },
          },
          {
            alert: 'KubeAggregatedAPIDown',
            expr: |||
              (1 - max by(name, namespace, %(clusterLabel)s)(avg_over_time(aggregator_unavailable_apiservice[10m]))) * 100 < 85
            ||| % $._config,
            'for': '5m',
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: 'Kubernetes aggregated API {{ $labels.name }}/{{ $labels.namespace }} has been only {{ $value | humanize }}% available over the last 10m.',
              summary: 'Kubernetes aggregated API is down.',
            },
          },
          (import '../lib/absent_alert.libsonnet') {
            componentName:: 'KubeAPI',
            selector:: $._config.kubeApiserverSelector,
          },
          {
            alert: 'KubeAPITerminatedRequests',
            expr: |||
              sum(rate(apiserver_request_terminations_total{%(kubeApiserverSelector)s}[10m]))  / (  sum(rate(apiserver_request_total{%(kubeApiserverSelector)s}[10m])) + sum(rate(apiserver_request_terminations_total{%(kubeApiserverSelector)s}[10m])) ) > 0.20
            ||| % $._config,
            labels: {
              severity: 'warning',
            },
            annotations: {
              description: 'The kubernetes apiserver has terminated {{ $value | humanizePercentage }} of its incoming requests.',
              summary: 'The kubernetes apiserver has terminated {{ $value | humanizePercentage }} of its incoming requests.',
            },
            'for': '5m',
          },
        ],
      },
    ],
  },
}
