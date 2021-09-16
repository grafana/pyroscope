local grafana = import 'grafonnet/grafana.libsonnet';

local width = 12;
local height = 10;

// Dashboard to be used by PRs
// IMPORTANT!
// Don't add rows, since that will mess up the json parsing
// in the golang code
grafana.dashboard.new(
  'Pyroscope Agent PR Dashboard',
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
      'Total Used Memory',
      datasource='$PROMETHEUS_DS',
      format='decbytes',
    )
    .addTarget(grafana.prometheus.target(
      'go_memstats_sys_bytes{instance=~"hotrod.*"}',
      legendFormat='{{ __name__ }}',
    )
  ),
  gridPos={
    x: width * 1,
    y: height * 2,
    w: width,
    h: height,
  }
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
      'rate(process_cpu_seconds_total{instance=~"hotrod.*"}[$__rate_interval])',
    )
  ),
  gridPos={
    x: width * 1,
    y: height * 2,
    w: width,
    h: height,
  }
)
.addPanel(
  grafana.graphPanel.new(
    'Goroutines',
    datasource='$PROMETHEUS_DS',
    legend_show=false,
  )
  .addTarget(grafana.prometheus.target('go_goroutines{instance=~"hotrod.*"}')),
  gridPos={
    x: width * 1,
    y: height * 3,
    w: width,
    h: height,
  }
)

