local grafana = import 'grafonnet/grafana.libsonnet';

local width = 12;
local height = 10;

// Dashboard to be used by Server benchmark PRs
grafana.dashboard.new(
  'Pyroscope Server PR Dashboard',
  tags=['pyroscope'],
  time_from='now-1h',
  uid='QF9YgRbUbt3BA5Qd',
  editable='true',
  refresh = '5s',
)
.addTemplate(
  grafana.template.datasource(
    name='PROMETHEUS_DS',
    query='prometheus',
    current='prometheus',
    hide='hidden', // anything other than '' and 'label works
  )
)
.addRow(
  grafana.row.new(
    title='Benchmark',
  )
  .addPanel(
    grafana.graphPanel.new(
      'Throughput',
      datasource='$PROMETHEUS_DS',
    )
    .addTarget(grafana.prometheus.target('rate(pyroscope_http_request_duration_seconds_count{handler="/ingest"}[5m])')),
  )
  .addPanel(
    grafana.graphPanel.new(
      'Disk Usage',
      datasource='$PROMETHEUS_DS',
      format='bytes',
      legend_values='true',
      legend_rightSide='true',
      legend_alignAsTable='true',
      legend_current='true',
      legend_sort='current',
      legend_sortDesc=true,
    )
    .addTarget(
      grafana.prometheus.target(
        'sum(pyroscope_storage_db_size_bytes) by (instance)',
        legendFormat='total {{ instance }}',
      )
    ),
  )
  .addPanel(
    grafana.graphPanel.new(
      'Memory',
      datasource='$PROMETHEUS_DS',
      format='bytes',
      legend_values='true',
      legend_rightSide='true',
      legend_alignAsTable='true',
      legend_current=true,
      legend_max=true,
      legend_sort='current',
      legend_sortDesc=true,
      logBase1Y=2,
    )
    .addTarget(
      grafana.prometheus.target(
        'go_memstats_heap_alloc_bytes{job="pyroscope"}',
        legendFormat='heap size {{ instance  }}',
      ),
    )
  )
  .addPanel(
    grafana.graphPanel.new(
      'Upload Errors (Total)',
      datasource='$PROMETHEUS_DS',
      span=4,
    )
    .addTarget(
      grafana.prometheus.target(
        'pyroscope_benchmark_upload_errors{}',
      )
    ),
  )
  .addPanel(
    grafana.graphPanel.new(
      'Successful Uploads (Total)',
      datasource='$PROMETHEUS_DS',
      span=4,
    )
    .addTarget(
      grafana.prometheus.target(
        'pyroscope_benchmark_successful_uploads{}',
      )
    ),
  )
  .addPanel(
    grafana.graphPanel.new(
      'CPU Utilization',
      datasource='$PROMETHEUS_DS',
      format='percent',
      min='0',
      max='100',
    )
    .addTarget(
      grafana.prometheus.target(
        'process_cpu_seconds_total{job="pyroscope"}',
      )
    )
  )
)
