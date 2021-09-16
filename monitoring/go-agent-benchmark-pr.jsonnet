local grafana = import 'grafonnet/grafana.libsonnet';

local width = 12;
local height = 10;
local selector='instance=~"hotrod.*"';

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
.addRow(
  grafana.row.new(title='Benchmark')
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
  )
  // seems cpu usage it's too low
  // so we using log base 2 here
  .addPanel(
    grafana.graphPanel.new(
      'CPU Utilization',
      datasource='$PROMETHEUS_DS',
      format='percent',
      max='100',
      logBase1Y=2,
    )
    .addTarget(
      grafana.prometheus.target(
        'rate(process_cpu_seconds_total{instance=~"hotrod.*"}[$__rate_interval])',
      )
    ),
  )
  .addPanel(
    grafana.graphPanel.new(
      'Goroutines',
      datasource='$PROMETHEUS_DS',
    )
    .addTarget(grafana.prometheus.target('go_goroutines{instance=~"hotrod.*"}')),
  )
)
.addRow(
  grafana.row.new(title='cAdvisor')
  .addPanel(
    grafana.graphPanel.new(
      'Container Memory',
      datasource='$PROMETHEUS_DS',
      format='decbytes',
    )
    .addTarget(grafana.prometheus.target(
      'container_memory_usage_bytes{container_label_com_docker_compose_service=~"hotrod.*"}'
    ))
    .addTarget(grafana.prometheus.target(
      'container_memory_working_set_bytes{container_label_com_docker_compose_service=~"hotrod.*"}'
    ))
    .addTarget(grafana.prometheus.target(
      'container_memory_rss{container_label_com_docker_compose_service=~"hotrod.*"}'
    ))
    .addTarget(grafana.prometheus.target(
      'container_memory_cache{container_label_com_docker_compose_service=~"hotrod.*"}'
    ))
    .addTarget(grafana.prometheus.target(
      'container_memory_mapped_file{container_label_com_docker_compose_service=~"hotrod.*"}'
    ))
    .addTarget(grafana.prometheus.target(
      'container_spec_memory_limit_bytes{container_label_com_docker_compose_service=~"hotrod.*"}'
    ))
  )
  .addPanel(
    grafana.graphPanel.new(
      'Container Network',
      datasource='$PROMETHEUS_DS',
      format='decbytes',
    )
    .addTarget(grafana.prometheus.target(
      'irate(container_network_receive_bytes_total{container_label_com_docker_compose_service=~"hotrod.*"}[$__rate_interval])',
      legendFormat='receive - {{ container_label_com_docker_compose_service }}'
    ))
    .addTarget(grafana.prometheus.target(
      'irate(container_network_transmit_bytes_total{container_label_com_docker_compose_service=~"hotrod.*"}[$__rate_interval])',
      legendFormat='transmit - {{ container_label_com_docker_compose_service }}'
    ))
  )
)

