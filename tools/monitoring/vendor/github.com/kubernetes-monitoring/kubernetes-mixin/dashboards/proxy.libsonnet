local grafana = import 'github.com/grafana/grafonnet-lib/grafonnet/grafana.libsonnet';
local dashboard = grafana.dashboard;
local row = grafana.row;
local prometheus = grafana.prometheus;
local template = grafana.template;
local graphPanel = grafana.graphPanel;
local singlestat = grafana.singlestat;

{
  grafanaDashboards+:: {
    'proxy.json':
      local upCount =
        singlestat.new(
          'Up',
          datasource='$datasource',
          span=2,
          valueName='min',
        )
        .addTarget(prometheus.target('sum(up{%(clusterLabel)s="$cluster", %(kubeProxySelector)s})' % $._config));

      local rulesSyncRate =
        graphPanel.new(
          'Rules Sync Rate',
          datasource='$datasource',
          span=5,
          min=0,
          format='ops',
        )
        .addTarget(prometheus.target('sum(rate(kubeproxy_sync_proxy_rules_duration_seconds_count{%(clusterLabel)s="$cluster", %(kubeProxySelector)s, instance=~"$instance"}[%(grafanaIntervalVar)s]))' % $._config, legendFormat='rate'));

      local rulesSyncLatency =
        graphPanel.new(
          'Rule Sync Latency 99th Quantile',
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
        .addTarget(prometheus.target('histogram_quantile(0.99,rate(kubeproxy_sync_proxy_rules_duration_seconds_bucket{%(clusterLabel)s="$cluster", %(kubeProxySelector)s, instance=~"$instance"}[%(grafanaIntervalVar)s]))' % $._config, legendFormat='{{instance}}'));

      local networkProgrammingRate =
        graphPanel.new(
          'Network Programming Rate',
          datasource='$datasource',
          span=6,
          min=0,
          format='ops',
        )
        .addTarget(prometheus.target('sum(rate(kubeproxy_network_programming_duration_seconds_count{%(clusterLabel)s="$cluster", %(kubeProxySelector)s, instance=~"$instance"}[%(grafanaIntervalVar)s]))' % $._config, legendFormat='rate'));

      local networkProgrammingLatency =
        graphPanel.new(
          'Network Programming Latency 99th Quantile',
          datasource='$datasource',
          span=6,
          min=0,
          format='s',
          legend_show=true,
          legend_values=true,
          legend_current=true,
          legend_alignAsTable=true,
          legend_rightSide=true,
        )
        .addTarget(prometheus.target('histogram_quantile(0.99, sum(rate(kubeproxy_network_programming_duration_seconds_bucket{%(clusterLabel)s="$cluster", %(kubeProxySelector)s, instance=~"$instance"}[%(grafanaIntervalVar)s])) by (instance, le))' % $._config, legendFormat='{{instance}}'));

      local rpcRate =
        graphPanel.new(
          'Kube API Request Rate',
          datasource='$datasource',
          span=4,
          format='ops',
        )
        .addTarget(prometheus.target('sum(rate(rest_client_requests_total{%(clusterLabel)s="$cluster", %(kubeProxySelector)s, instance=~"$instance",code=~"2.."}[%(grafanaIntervalVar)s]))' % $._config, legendFormat='2xx'))
        .addTarget(prometheus.target('sum(rate(rest_client_requests_total{%(clusterLabel)s="$cluster", %(kubeProxySelector)s, instance=~"$instance",code=~"3.."}[%(grafanaIntervalVar)s]))' % $._config, legendFormat='3xx'))
        .addTarget(prometheus.target('sum(rate(rest_client_requests_total{%(clusterLabel)s="$cluster", %(kubeProxySelector)s, instance=~"$instance",code=~"4.."}[%(grafanaIntervalVar)s]))' % $._config, legendFormat='4xx'))
        .addTarget(prometheus.target('sum(rate(rest_client_requests_total{%(clusterLabel)s="$cluster", %(kubeProxySelector)s, instance=~"$instance",code=~"5.."}[%(grafanaIntervalVar)s]))' % $._config, legendFormat='5xx'));

      local postRequestLatency =
        graphPanel.new(
          'Post Request Latency 99th Quantile',
          datasource='$datasource',
          span=8,
          format='s',
          min=0,
        )
        .addTarget(prometheus.target('histogram_quantile(0.99, sum(rate(rest_client_request_duration_seconds_bucket{%(clusterLabel)s="$cluster", %(kubeProxySelector)s,instance=~"$instance",verb="POST"}[%(grafanaIntervalVar)s])) by (verb, url, le))' % $._config, legendFormat='{{verb}} {{url}}'));

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
        .addTarget(prometheus.target('histogram_quantile(0.99, sum(rate(rest_client_request_duration_seconds_bucket{%(clusterLabel)s="$cluster", %(kubeProxySelector)s, instance=~"$instance", verb="GET"}[%(grafanaIntervalVar)s])) by (verb, url, le))' % $._config, legendFormat='{{verb}} {{url}}'));

      local memory =
        graphPanel.new(
          'Memory',
          datasource='$datasource',
          span=4,
          format='bytes',
        )
        .addTarget(prometheus.target('process_resident_memory_bytes{%(clusterLabel)s="$cluster", %(kubeProxySelector)s,instance=~"$instance"}' % $._config, legendFormat='{{instance}}'));

      local cpu =
        graphPanel.new(
          'CPU usage',
          datasource='$datasource',
          span=4,
          format='short',
          min=0,
        )
        .addTarget(prometheus.target('rate(process_cpu_seconds_total{%(clusterLabel)s="$cluster", %(kubeProxySelector)s,instance=~"$instance"}[%(grafanaIntervalVar)s])' % $._config, legendFormat='{{instance}}'));

      local goroutines =
        graphPanel.new(
          'Goroutines',
          datasource='$datasource',
          span=4,
          format='short',
        )
        .addTarget(prometheus.target('go_goroutines{%(clusterLabel)s="$cluster", %(kubeProxySelector)s,instance=~"$instance"}' % $._config, legendFormat='{{instance}}'));


      dashboard.new(
        '%(dashboardNamePrefix)sProxy' % $._config.grafanaK8s,
        time_from='now-1h',
        uid=($._config.grafanaDashboardIDs['proxy.json']),
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
          'label_values(up{%(kubeProxySelector)s}, %(clusterLabel)s)' % $._config,
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
          'label_values(up{%(kubeProxySelector)s, %(clusterLabel)s="$cluster", %(kubeProxySelector)s}, instance)' % $._config,
          refresh='time',
          includeAll=true,
          sort=1,
        )
      )
      .addRow(
        row.new()
        .addPanel(upCount)
        .addPanel(rulesSyncRate)
        .addPanel(rulesSyncLatency)
      ).addRow(
        row.new()
        .addPanel(networkProgrammingRate)
        .addPanel(networkProgrammingLatency)
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
