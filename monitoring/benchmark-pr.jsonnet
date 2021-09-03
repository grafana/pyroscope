local grafana = import 'grafonnet/grafana.libsonnet';

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
    w: 12,
    h: 10,
  }
)
