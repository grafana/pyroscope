local grafana = import 'grafonnet/grafana.libsonnet';


{
  dashboard:
    local d = grafana.dashboard.new(
      'Pyroscope Server Comparison',
      tags=['pyroscope'],
      time_from='now-30m',
      uid='8mDG2MCwXqg9hTPT',
      editable='true',
      refresh = '5s',
    );


    d
    .addTemplate(
      grafana.template.datasource(
        name='PROMETHEUS_DS',
        query='prometheus',
        current='prometheus',
        hide='hidden',  // anything other than '' and 'label works
      )
    )
    .addTemplate(
      grafana.template.new(
        'instance',
        '$PROMETHEUS_DS',
        'label_values(pyroscope_build_info, instance)',
        // otherwise the variable may be unpopulated
        // eg. when prometheus/grafana/pyroscope are started at the same time
        refresh='time',
        label='instance',
        includeAll=if $._config.benchmark then true else false,
      )
    )

    .addRow(
      grafana.row.new(
        title='Meta',
      )
      .addPanel(
        grafana.tablePanel.new(
          title='',
          datasource='$PROMETHEUS_DS',
          span=12,
          height=10,
        )
        // they don't provide any value
        .hideColumn("__name__")
        .hideColumn("Time")
        .hideColumn("Value")
        .hideColumn("job")

        // somewhat useful but preferred to be hidden
        // to make the table cleaner
        .hideColumn("use_embedded_assets")
        .addTarget(
          grafana.prometheus.target(
            'pyroscope_build_info{%s}' % $._config.selector,
            instant=true,
            format='table',
          )
        )
      )
    )


    // Only useful when running benchmark


    .addRow(
      grafana.row.new(
        title='General',
      )
      .addPanel(
        grafana.graphPanel.new(
          'Request Latency P99',
          datasource='$PROMETHEUS_DS',
          format='seconds',
        )
        .addTarget(grafana.prometheus.target(|||
            histogram_quantile(0.99,
              sum(rate(pyroscope_http_request_duration_seconds_bucket{
                instance="$instance",
                handler!="/metrics",
                handler!="/healthz"
              }[$__rate_interval]))
              by (le, handler)
            )
          |||,
          legendFormat='{{ handler }}',
        ))
      )

      .addPanel(
        grafana.graphPanel.new(
          'Error Rate',
          datasource='$PROMETHEUS_DS',
        )
        .addTarget(grafana.prometheus.target(|||
          sum(rate(pyroscope_http_request_duration_seconds_count
          {instance="$instance", code=~"5..", handler!="/metrics", handler!="/healthz"}[$__rate_interval])) by (handler)
          /
          sum(rate(pyroscope_http_request_duration_seconds_count{instance="$instance", handler!="/metrics", handler!="/healthz"}[$__rate_interval])) by (handler)
        |||,
          legendFormat='{{ handler }}',
        ))
      )

      .addPanel(
        grafana.graphPanel.new(
          'Throughput',
          datasource='$PROMETHEUS_DS',
        )
        .addTarget(grafana.prometheus.target('sum(rate(pyroscope_http_request_duration_seconds_count{instance="$instance", handler!="/metrics", handler!="/healthz"}[$__rate_interval])) by (handler)',
          legendFormat='{{ handler }}',
        ))
      )

      .addPanel(
        grafana.graphPanel.new(
          'Response Size P99',
          datasource='$PROMETHEUS_DS',
          format='bytes',
        )
        .addTarget(grafana.prometheus.target('histogram_quantile(0.95, sum(rate(pyroscope_http_response_size_bytes_bucket{instance="$instance", handler!="/metrics", handler!="/healthz"}[$__rate_interval])) by (le, handler))',
          legendFormat='{{ handler }}',
        ))
      )

      .addPanel(
        grafana.graphPanel.new(
          'CPU Utilization',
          datasource='$PROMETHEUS_DS',
          format='percent',
          min='0',
          max='100',
          legend_show=false,
        )
        .addTarget(
          grafana.prometheus.target(
            'process_cpu_seconds_total{instance="$instance"}',
          )
        )
      )

    )
}
