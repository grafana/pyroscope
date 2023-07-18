local grafana = import 'github.com/grafana/grafonnet-lib/grafonnet/grafana.libsonnet';
local dashboard = grafana.dashboard;
local row = grafana.row;
local prometheus = grafana.prometheus;
local template = grafana.template;
local graphPanel = grafana.graphPanel;
local singlestat = grafana.singlestat;

{
  grafanaDashboards+:: {
    'scheduler.json':
      local upCount =
        singlestat.new(
          'Up',
          datasource='$datasource',
          span=2,
          valueName='min',
        )
        .addTarget(prometheus.target('sum(up{%(clusterLabel)s="$cluster", %(kubeSchedulerSelector)s})' % $._config));

      local schedulingRate =
        graphPanel.new(
          'Scheduling Rate',
          datasource='$datasource',
          span=5,
          format='ops',
          min=0,
          legend_show=true,
          legend_values=true,
          legend_current=true,
          legend_alignAsTable=true,
          legend_rightSide=true,
        )
        .addTarget(prometheus.target('sum(rate(scheduler_e2e_scheduling_duration_seconds_count{%(clusterLabel)s="$cluster", %(kubeSchedulerSelector)s, instance=~"$instance"}[%(grafanaIntervalVar)s])) by (%(clusterLabel)s, instance)' % $._config, legendFormat='{{%(clusterLabel)s}} {{instance}} e2e' % $._config))
        .addTarget(prometheus.target('sum(rate(scheduler_binding_duration_seconds_count{%(clusterLabel)s="$cluster", %(kubeSchedulerSelector)s, instance=~"$instance"}[%(grafanaIntervalVar)s])) by (%(clusterLabel)s, instance)' % $._config, legendFormat='{{%(clusterLabel)s}} {{instance}} binding' % $._config))
        .addTarget(prometheus.target('sum(rate(scheduler_scheduling_algorithm_duration_seconds_count{%(clusterLabel)s="$cluster", %(kubeSchedulerSelector)s, instance=~"$instance"}[%(grafanaIntervalVar)s])) by (%(clusterLabel)s, instance)' % $._config, legendFormat='{{%(clusterLabel)s}} {{instance}} scheduling algorithm' % $._config))
        .addTarget(prometheus.target('sum(rate(scheduler_volume_scheduling_duration_seconds_count{%(clusterLabel)s="$cluster", %(kubeSchedulerSelector)s, instance=~"$instance"}[%(grafanaIntervalVar)s])) by (%(clusterLabel)s, instance)' % $._config, legendFormat='{{%(clusterLabel)s}} {{instance}} volume' % $._config));


      local schedulingLatency =
        graphPanel.new(
          'Scheduling latency 99th Quantile',
          datasource='$datasource',
          span=5,
          min=0,
          format='s',
          legend_show=true,
          legend_values=true,
          legend_current=true,
          legend_alignAsTable=true,
          legend_rightSide=true,
        )
        .addTarget(prometheus.target('histogram_quantile(0.99, sum(rate(scheduler_e2e_scheduling_duration_seconds_bucket{%(clusterLabel)s="$cluster", %(kubeSchedulerSelector)s,instance=~"$instance"}[%(grafanaIntervalVar)s])) by (%(clusterLabel)s, instance, le))' % $._config, legendFormat='{{%(clusterLabel)s}} {{instance}} e2e' % $._config))
        .addTarget(prometheus.target('histogram_quantile(0.99, sum(rate(scheduler_binding_duration_seconds_bucket{%(clusterLabel)s="$cluster", %(kubeSchedulerSelector)s,instance=~"$instance"}[%(grafanaIntervalVar)s])) by (%(clusterLabel)s, instance, le))' % $._config, legendFormat='{{%(clusterLabel)s}} {{instance}} binding' % $._config))
        .addTarget(prometheus.target('histogram_quantile(0.99, sum(rate(scheduler_scheduling_algorithm_duration_seconds_bucket{%(clusterLabel)s="$cluster", %(kubeSchedulerSelector)s,instance=~"$instance"}[%(grafanaIntervalVar)s])) by (%(clusterLabel)s, instance, le))' % $._config, legendFormat='{{%(clusterLabel)s}} {{instance}} scheduling algorithm' % $._config))
        .addTarget(prometheus.target('histogram_quantile(0.99, sum(rate(scheduler_volume_scheduling_duration_seconds_bucket{%(clusterLabel)s="$cluster", %(kubeSchedulerSelector)s,instance=~"$instance"}[%(grafanaIntervalVar)s])) by (%(clusterLabel)s, instance, le))' % $._config, legendFormat='{{%(clusterLabel)s}} {{instance}} volume' % $._config));

      local rpcRate =
        graphPanel.new(
          'Kube API Request Rate',
          datasource='$datasource',
          span=4,
          format='ops',
          min=0,
        )
        .addTarget(prometheus.target('sum(rate(rest_client_requests_total{%(clusterLabel)s="$cluster", %(kubeSchedulerSelector)s, instance=~"$instance",code=~"2.."}[%(grafanaIntervalVar)s]))' % $._config, legendFormat='2xx'))
        .addTarget(prometheus.target('sum(rate(rest_client_requests_total{%(clusterLabel)s="$cluster", %(kubeSchedulerSelector)s, instance=~"$instance",code=~"3.."}[%(grafanaIntervalVar)s]))' % $._config, legendFormat='3xx'))
        .addTarget(prometheus.target('sum(rate(rest_client_requests_total{%(clusterLabel)s="$cluster", %(kubeSchedulerSelector)s, instance=~"$instance",code=~"4.."}[%(grafanaIntervalVar)s]))' % $._config, legendFormat='4xx'))
        .addTarget(prometheus.target('sum(rate(rest_client_requests_total{%(clusterLabel)s="$cluster", %(kubeSchedulerSelector)s, instance=~"$instance",code=~"5.."}[%(grafanaIntervalVar)s]))' % $._config, legendFormat='5xx'));

      local postRequestLatency =
        graphPanel.new(
          'Post Request Latency 99th Quantile',
          datasource='$datasource',
          span=8,
          format='s',
          min=0,
        )
        .addTarget(prometheus.target('histogram_quantile(0.99, sum(rate(rest_client_request_duration_seconds_bucket{%(clusterLabel)s="$cluster", %(kubeSchedulerSelector)s, instance=~"$instance", verb="POST"}[%(grafanaIntervalVar)s])) by (verb, url, le))' % $._config, legendFormat='{{verb}} {{url}}'));

      local getRequestLatency =
        graphPanel.new(
          'Get Request Latency 99th Quantile',
          datasource='$datasource',
          span=12,
          format='s',
          min=0,
          legend_show=true,
          legend_values=true,
          legend_current=true,
          legend_alignAsTable=true,
          legend_rightSide=true,
        )
        .addTarget(prometheus.target('histogram_quantile(0.99, sum(rate(rest_client_request_duration_seconds_bucket{%(clusterLabel)s="$cluster", %(kubeSchedulerSelector)s, instance=~"$instance", verb="GET"}[%(grafanaIntervalVar)s])) by (verb, url, le))' % $._config, legendFormat='{{verb}} {{url}}'));

      local memory =
        graphPanel.new(
          'Memory',
          datasource='$datasource',
          span=4,
          format='bytes',
        )
        .addTarget(prometheus.target('process_resident_memory_bytes{%(clusterLabel)s="$cluster", %(kubeSchedulerSelector)s, instance=~"$instance"}' % $._config, legendFormat='{{instance}}'));

      local cpu =
        graphPanel.new(
          'CPU usage',
          datasource='$datasource',
          span=4,
          format='bytes',
          min=0,
        )
        .addTarget(prometheus.target('rate(process_cpu_seconds_total{%(clusterLabel)s="$cluster", %(kubeSchedulerSelector)s, instance=~"$instance"}[%(grafanaIntervalVar)s])' % $._config, legendFormat='{{instance}}'));

      local goroutines =
        graphPanel.new(
          'Goroutines',
          datasource='$datasource',
          span=4,
          format='short',
        )
        .addTarget(prometheus.target('go_goroutines{%(clusterLabel)s="$cluster", %(kubeSchedulerSelector)s,instance=~"$instance"}' % $._config, legendFormat='{{instance}}'));


      dashboard.new(
        '%(dashboardNamePrefix)sScheduler' % $._config.grafanaK8s,
        time_from='now-1h',
        uid=($._config.grafanaDashboardIDs['scheduler.json']),
        tags=($._config.grafanaK8s.dashboardTags),
      ).addTemplate(
        {
          current: {
            text: 'default',
            value: $._config.datasourceName,
          },
          hide: 0,
          label: 'Data Source',
          name: 'datasource',
          options: [],
          query: 'prometheus',
          refresh: 1,
          regex: $._config.datasourceFilterRegex,
          type: 'datasource',
        },
      )
      .addTemplate(
        template.new(
          'cluster',
          '$datasource',
          'label_values(up{%(kubeSchedulerSelector)s}, %(clusterLabel)s)' % $._config,
          label='cluster',
          refresh='time',
          hide=if $._config.showMultiCluster then '' else 'variable',
          sort=1,
        )
      )
      .addTemplate(
        template.new(
          'instance',
          '$datasource',
          'label_values(up{%(kubeSchedulerSelector)s, %(clusterLabel)s="$cluster"}, instance)' % $._config,
          refresh='time',
          includeAll=true,
          sort=1,
        )
      )
      .addRow(
        row.new()
        .addPanel(upCount)
        .addPanel(schedulingRate)
        .addPanel(schedulingLatency)
      ).addRow(
        row.new()
        .addPanel(rpcRate)
        .addPanel(postRequestLatency)
      ).addRow(
        row.new()
        .addPanel(getRequestLatency)
      ).addRow(
        row.new()
        .addPanel(memory)
        .addPanel(cpu)
        .addPanel(goroutines)
      ),
  },
}
