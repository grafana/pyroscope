{
  prometheusAlerts+:: {
    groups+: [
      {
        name: 'alertmanager.rules',
        rules: [
          {
            alert: 'AlertmanagerFailedReload',
            expr: |||
              # Without max_over_time, failed scrapes could create false negatives, see
              # https://www.robustperception.io/alerting-on-gauges-in-prometheus-2-0 for details.
              max_over_time(alertmanager_config_last_reload_successful{%(alertmanagerSelector)s}[5m]) == 0
            ||| % $._config,
            'for': '10m',
            labels: {
              severity: 'critical',
            },
            annotations: {
              summary: 'Reloading an Alertmanager configuration has failed.',
              description: 'Configuration has failed to load for %(alertmanagerName)s.' % $._config,
            },
          },
          {
            alert: 'AlertmanagerMembersInconsistent',
            expr: |||
              # Without max_over_time, failed scrapes could create false negatives, see
              # https://www.robustperception.io/alerting-on-gauges-in-prometheus-2-0 for details.
                max_over_time(alertmanager_cluster_members{%(alertmanagerSelector)s}[5m])
              < on (%(alertmanagerClusterLabels)s) group_left
                count by (%(alertmanagerClusterLabels)s) (max_over_time(alertmanager_cluster_members{%(alertmanagerSelector)s}[5m]))
            ||| % $._config,
            'for': '15m',
            labels: {
              severity: 'critical',
            },
            annotations: {
              summary: 'A member of an Alertmanager cluster has not found all other cluster members.',
              description: 'Alertmanager %(alertmanagerName)s has only found {{ $value }} members of the %(alertmanagerClusterName)s cluster.' % $._config,
            },
          },
          {
            alert: 'AlertmanagerFailedToSendAlerts',
            expr: |||
              (
                rate(alertmanager_notifications_failed_total{%(alertmanagerSelector)s}[5m])
              /
                rate(alertmanager_notifications_total{%(alertmanagerSelector)s}[5m])
              )
              > 0.01
            ||| % $._config,
            'for': '5m',
            labels: {
              severity: 'warning',
            },
            annotations: {
              summary: 'An Alertmanager instance failed to send notifications.',
              description: 'Alertmanager %(alertmanagerName)s failed to send {{ $value | humanizePercentage }} of notifications to {{ $labels.integration }}.' % $._config,
            },
          },
          {
            alert: 'AlertmanagerClusterFailedToSendAlerts',
            expr: |||
              min by (%(alertmanagerClusterLabels)s, integration) (
                rate(alertmanager_notifications_failed_total{%(alertmanagerSelector)s, integration=~`%(alertmanagerCriticalIntegrationsRegEx)s`}[5m])
              /
                rate(alertmanager_notifications_total{%(alertmanagerSelector)s, integration=~`%(alertmanagerCriticalIntegrationsRegEx)s`}[5m])
              )
              > 0.01
            ||| % $._config,
            'for': '5m',
            labels: {
              severity: 'critical',
            },
            annotations: {
              summary: 'All Alertmanager instances in a cluster failed to send notifications to a critical integration.',
              description: 'The minimum notification failure rate to {{ $labels.integration }} sent from any instance in the %(alertmanagerClusterName)s cluster is {{ $value | humanizePercentage }}.' % $._config,
            },
          },
          {
            alert: 'AlertmanagerClusterFailedToSendAlerts',
            expr: |||
              min by (%(alertmanagerClusterLabels)s, integration) (
                rate(alertmanager_notifications_failed_total{%(alertmanagerSelector)s, integration!~`%(alertmanagerCriticalIntegrationsRegEx)s`}[5m])
              /
                rate(alertmanager_notifications_total{%(alertmanagerSelector)s, integration!~`%(alertmanagerCriticalIntegrationsRegEx)s`}[5m])
              )
              > 0.01
            ||| % $._config,
            'for': '5m',
            labels: {
              severity: 'warning',
            },
            annotations: {
              summary: 'All Alertmanager instances in a cluster failed to send notifications to a non-critical integration.',
              description: 'The minimum notification failure rate to {{ $labels.integration }} sent from any instance in the %(alertmanagerClusterName)s cluster is {{ $value | humanizePercentage }}.' % $._config,
            },
          },
          {
            alert: 'AlertmanagerConfigInconsistent',
            expr: |||
              count by (%(alertmanagerClusterLabels)s) (
                count_values by (%(alertmanagerClusterLabels)s) ("config_hash", alertmanager_config_hash{%(alertmanagerSelector)s})
              )
              != 1
            ||| % $._config,
            'for': '20m',  // A config change across an Alertmanager cluster can take its time. But it's really bad if it persists for too long.
            labels: {
              severity: 'critical',
            },
            annotations: {
              summary: 'Alertmanager instances within the same cluster have different configurations.',
              description: 'Alertmanager instances within the %(alertmanagerClusterName)s cluster have different configurations.' % $._config,
            },
          },
          // Both the following critical alerts, AlertmanagerClusterDown and
          // AlertmanagerClusterCrashlooping, fire if a whole cluster is
          // unhealthy. It is implied that a generic warning alert is in place
          // for individual instances being down or crashlooping.
          {
            alert: 'AlertmanagerClusterDown',
            expr: |||
              (
                count by (%(alertmanagerClusterLabels)s) (
                  avg_over_time(up{%(alertmanagerSelector)s}[5m]) < 0.5
                )
              /
                count by (%(alertmanagerClusterLabels)s) (
                  up{%(alertmanagerSelector)s}
                )
              )
              >= 0.5
            ||| % $._config,
            'for': '5m',
            labels: {
              severity: 'critical',
            },
            annotations: {
              summary: 'Half or more of the Alertmanager instances within the same cluster are down.',
              description: '{{ $value | humanizePercentage }} of Alertmanager instances within the %(alertmanagerClusterName)s cluster have been up for less than half of the last 5m.' % $._config,
            },
          },
          {
            alert: 'AlertmanagerClusterCrashlooping',
            expr: |||
              (
                count by (%(alertmanagerClusterLabels)s) (
                  changes(process_start_time_seconds{%(alertmanagerSelector)s}[10m]) > 4
                )
              /
                count by (%(alertmanagerClusterLabels)s) (
                  up{%(alertmanagerSelector)s}
                )
              )
              >= 0.5
            ||| % $._config,
            'for': '5m',
            labels: {
              severity: 'critical',
            },
            annotations: {
              summary: 'Half or more of the Alertmanager instances within the same cluster are crashlooping.',
              description: '{{ $value | humanizePercentage }} of Alertmanager instances within the %(alertmanagerClusterName)s cluster have restarted at least 5 times in the last 10m.' % $._config,
            },
          },
        ],
      },
    ],
  },
}
