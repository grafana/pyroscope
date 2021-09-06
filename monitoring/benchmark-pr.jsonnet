local grafana = import 'grafonnet/grafana.libsonnet';

local width = 12;
local height = 10;
// Dashboard to be used by PRs
// IMPORTANT!
// Don't add rows, since that will mess up the json parsing
// in the golang code
grafana.dashboard.new(
  'Pyroscope PR Dashboard',
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
.addPanel(
  grafana.graphPanel.new(
    'Throughput',
    datasource='$PROMETHEUS_DS',
  )
  .addTarget(grafana.prometheus.target('rate(pyroscope_http_request_duration_seconds_count{handler="/ingest"}[5m])')),
  gridPos={
    x: width * 0,
    y: height * 0,
    w: width,
    h: height,
  }
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
      'sum(pyroscope_storage_disk_bytes) by (instance)',
      legendFormat='total {{ instance }}',
    )
  ),
  gridPos={
    x: width * 1,
    y: height * 0,
    w: width,
    h: height,
  }
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
      'pyroscope_storage_evictions_alloc_bytes',
      legendFormat='heap size {{ instance  }}',
    ),
  )
  .addTarget(
    grafana.prometheus.target(
      'pyroscope_storage_evictions_total_mem_bytes',
      legendFormat='total memory {{ instance }}',
    ),
  ),
  gridPos={
    x: width * 0,
    y: height * 1,
    w: width,
    h: height,
  }
)
