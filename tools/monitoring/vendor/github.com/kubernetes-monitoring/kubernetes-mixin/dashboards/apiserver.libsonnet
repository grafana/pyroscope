local grafana = import 'github.com/grafana/grafonnet-lib/grafonnet/grafana.libsonnet';
local dashboard = grafana.dashboard;
local row = grafana.row;
local prometheus = grafana.prometheus;
local template = grafana.template;
local graphPanel = grafana.graphPanel;
local singlestat = grafana.singlestat;

{
  _config+:: {
    kubeApiserverSelector: 'job="kube-apiserver"',
  },

  grafanaDashboards+:: {
    'apiserver.json':
      local availability1d =
        singlestat.new(
          'Availability (%dd) > %.3f%%' % [$._config.SLOs.apiserver.days, 100 * $._config.SLOs.apiserver.target],
          datasource='$datasource',
          span=4,
          format='percentunit',
          decimals=3,
          description='How many percent of requests (both read and write) in %d days have been answered successfully and fast enough?' % $._config.SLOs.apiserver.days,
        )
        .addTarget(prometheus.target('apiserver_request:availability%dd{verb="all", %(clusterLabel)s="$cluster"}' % [$._config.SLOs.apiserver.days, $._config.clusterLabel]));

      local errorBudget =
        graphPanel.new(
          'ErrorBudget (%dd) > %.3f%%' % [$._config.SLOs.apiserver.days, 100 * $._config.SLOs.apiserver.target],
          datasource='$datasource',
          span=8,
          format='percentunit',
          decimals=3,
          fill=10,
          description='How much error budget is left looking at our %.3f%% availability guarantees?' % $._config.SLOs.apiserver.target,
        )
        .addTarget(prometheus.target('100 * (apiserver_request:availability%dd{verb="all", %(clusterLabel)s="$cluster"} - %f)' % [$._config.SLOs.apiserver.days, $._config.clusterLabel, $._config.SLOs.apiserver.target], legendFormat='errorbudget'));

      local readAvailability =
        singlestat.new(
          'Read Availability (%dd)' % $._config.SLOs.apiserver.days,
          datasource='$datasource',
          span=3,
          format='percentunit',
          decimals=3,
          description='How many percent of read requests (LIST,GET) in %d days have been answered successfully and fast enough?' % $._config.SLOs.apiserver.days,
        )
        .addTarget(prometheus.target('apiserver_request:availability%dd{verb="read", %(clusterLabel)s="$cluster"}' % [$._config.SLOs.apiserver.days, $._config.clusterLabel]));

      local readRequests =
        graphPanel.new(
          'Read SLI - Requests',
          datasource='$datasource',
          span=3,
          format='reqps',
          stack=true,
          fill=10,
          description='How many read requests (LIST,GET) per second do the apiservers get by code?',
        )
        .addSeriesOverride({ alias: '/2../i', color: '#56A64B' })
        .addSeriesOverride({ alias: '/3../i', color: '#F2CC0C' })
        .addSeriesOverride({ alias: '/4../i', color: '#3274D9' })
        .addSeriesOverride({ alias: '/5../i', color: '#E02F44' })
        .addTarget(prometheus.target('sum by (code) (code_resource:apiserver_request_total:rate5m{verb="read", %(clusterLabel)s="$cluster"})' % $._config, legendFormat='{{ code }}'));

      local readErrors =
        graphPanel.new(
          'Read SLI - Errors',
          datasource='$datasource',
          min=0,
          span=3,
          format='percentunit',
          description='How many percent of read requests (LIST,GET) per second are returned with errors (5xx)?',
        )
        .addTarget(prometheus.target('sum by (resource) (code_resource:apiserver_request_total:rate5m{verb="read",code=~"5..", %(clusterLabel)s="$cluster"}) / sum by (resource) (code_resource:apiserver_request_total:rate5m{verb="read", %(clusterLabel)s="$cluster"})' % $._config, legendFormat='{{ resource }}'));

      local readDuration =
        graphPanel.new(
          'Read SLI - Duration',
          datasource='$datasource',
          span=3,
          format='s',
          description='How many seconds is the 99th percentile for reading (LIST|GET) a given resource?',
        )
        .addTarget(prometheus.target('cluster_quantile:apiserver_request_slo_duration_seconds:histogram_quantile{verb="read", %(clusterLabel)s="$cluster"}' % $._config, legendFormat='{{ resource }}'));

      local writeAvailability =
        singlestat.new(
          'Write Availability (%dd)' % $._config.SLOs.apiserver.days,
          datasource='$datasource',
          span=3,
          format='percentunit',
          decimals=3,
          description='How many percent of write requests (POST|PUT|PATCH|DELETE) in %d days have been answered successfully and fast enough?' % $._config.SLOs.apiserver.days,
        )
        .addTarget(prometheus.target('apiserver_request:availability%dd{verb="write", %(clusterLabel)s="$cluster"}' % [$._config.SLOs.apiserver.days, $._config.clusterLabel]));

      local writeRequests =
        graphPanel.new(
          'Write SLI - Requests',
          datasource='$datasource',
          span=3,
          format='reqps',
          stack=true,
          fill=10,
          description='How many write requests (POST|PUT|PATCH|DELETE) per second do the apiservers get by code?',
        )
        .addSeriesOverride({ alias: '/2../i', color: '#56A64B' })
        .addSeriesOverride({ alias: '/3../i', color: '#F2CC0C' })
        .addSeriesOverride({ alias: '/4../i', color: '#3274D9' })
        .addSeriesOverride({ alias: '/5../i', color: '#E02F44' })
        .addTarget(prometheus.target('sum by (code) (code_resource:apiserver_request_total:rate5m{verb="write", %(clusterLabel)s="$cluster"})' % $._config, legendFormat='{{ code }}'));

      local writeErrors =
        graphPanel.new(
          'Write SLI - Errors',
          datasource='$datasource',
          min=0,
          span=3,
          format='percentunit',
          description='How many percent of write requests (POST|PUT|PATCH|DELETE) per second are returned with errors (5xx)?',
        )
        .addTarget(prometheus.target('sum by (resource) (code_resource:apiserver_request_total:rate5m{verb="write",code=~"5..", %(clusterLabel)s="$cluster"}) / sum by (resource) (code_resource:apiserver_request_total:rate5m{verb="write", %(clusterLabel)s="$cluster"})' % $._config, legendFormat='{{ resource }}'));

      local writeDuration =
        graphPanel.new(
          'Write SLI - Duration',
          datasource='$datasource',
          span=3,
          format='s',
          description='How many seconds is the 99th percentile for writing (POST|PUT|PATCH|DELETE) a given resource?',
        )
        .addTarget(prometheus.target('cluster_quantile:apiserver_request_slo_duration_seconds:histogram_quantile{verb="write", %(clusterLabel)s="$cluster"}' % $._config, legendFormat='{{ resource }}'));

      local workQueueAddRate =
        graphPanel.new(
          'Work Queue Add Rate',
          datasource='$datasource',
          span=6,
          format='ops',
          legend_show=false,
          min=0,
        )
        .addTarget(prometheus.target('sum(rate(workqueue_adds_total{%(kubeApiserverSelector)s, instance=~"$instance", %(clusterLabel)s="$cluster"}[%(grafanaIntervalVar)s])) by (instance, name)' % $._config, legendFormat='{{instance}} {{name}}'));

      local workQueueDepth =
        graphPanel.new(
          'Work Queue Depth',
          datasource='$datasource',
          span=6,
          format='short',
          legend_show=false,
          min=0,
        )
        .addTarget(prometheus.target('sum(rate(workqueue_depth{%(kubeApiserverSelector)s, instance=~"$instance", %(clusterLabel)s="$cluster"}[%(grafanaIntervalVar)s])) by (instance, name)' % $._config, legendFormat='{{instance}} {{name}}'));


      local workQueueLatency =
        graphPanel.new(
          'Work Queue Latency',
          datasource='$datasource',
          span=12,
          format='s',
          legend_show=true,
          legend_values=true,
          legend_current=true,
          legend_alignAsTable=true,
          legend_rightSide=true,
        )
        .addTarget(prometheus.target('histogram_quantile(0.99, sum(rate(workqueue_queue_duration_seconds_bucket{%(kubeApiserverSelector)s, instance=~"$instance", %(clusterLabel)s="$cluster"}[%(grafanaIntervalVar)s])) by (instance, name, le))' % $._config, legendFormat='{{instance}} {{name}}'));

      local memory =
        graphPanel.new(
          'Memory',
          datasource='$datasource',
          span=4,
          format='bytes',
        )
        .addTarget(prometheus.target('process_resident_memory_bytes{%(kubeApiserverSelector)s,instance=~"$instance", %(clusterLabel)s="$cluster"}' % $._config, legendFormat='{{instance}}'));

      local cpu =
        graphPanel.new(
          'CPU usage',
          datasource='$datasource',
          span=4,
          format='short',
          min=0,
        )
        .addTarget(prometheus.target('rate(process_cpu_seconds_total{%(kubeApiserverSelector)s,instance=~"$instance", %(clusterLabel)s="$cluster"}[%(grafanaIntervalVar)s])' % $._config, legendFormat='{{instance}}'));

      local goroutines =
        graphPanel.new(
          'Goroutines',
          datasource='$datasource',
          span=4,
          format='short',
        )
        .addTarget(prometheus.target('go_goroutines{%(kubeApiserverSelector)s,instance=~"$instance", %(clusterLabel)s="$cluster"}' % $._config, legendFormat='{{instance}}'));

      dashboard.new(
        '%(dashboardNamePrefix)sAPI server' % $._config.grafanaK8s,
        time_from='now-1h',
        uid=($._config.grafanaDashboardIDs['apiserver.json']),
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
          'label_values(up{%(kubeApiserverSelector)s}, %(clusterLabel)s)' % $._config,
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
          'label_values(up{%(kubeApiserverSelector)s, %(clusterLabel)s="$cluster"}, instance)' % $._config,
          refresh='time',
          includeAll=true,
          sort=1,
        )
      )
      .addPanel(
        grafana.text.new(
          title='Notice',
          content='The SLO (service level objective) and other metrics displayed on this dashboard are for informational purposes only.',
          description='The SLO (service level objective) and other metrics displayed on this dashboard are for informational purposes only.',
          span=12,
        ),
        gridPos={
          h: 2,
          w: 24,
          x: 0,
          y: 0,
        },
      )
      .addRow(
        row.new()
        .addPanel(availability1d)
        .addPanel(errorBudget)
      )
      .addRow(
        row.new()
        .addPanel(readAvailability)
        .addPanel(readRequests)
        .addPanel(readErrors)
        .addPanel(readDuration)
      )
      .addRow(
        row.new()
        .addPanel(writeAvailability)
        .addPanel(writeRequests)
        .addPanel(writeErrors)
        .addPanel(writeDuration)
      ).addRow(
        row.new()
        .addPanel(workQueueAddRate)
        .addPanel(workQueueDepth)
        .addPanel(workQueueLatency)
      ).addRow(
        row.new()
        .addPanel(memory)
        .addPanel(cpu)
        .addPanel(goroutines)
      ),
  },
}
