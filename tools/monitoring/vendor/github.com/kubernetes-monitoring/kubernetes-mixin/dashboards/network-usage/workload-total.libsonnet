local grafana = import 'github.com/grafana/grafonnet-lib/grafonnet/grafana.libsonnet';
local dashboard = grafana.dashboard;
local row = grafana.row;
local prometheus = grafana.prometheus;
local template = grafana.template;
local graphPanel = grafana.graphPanel;
local annotation = grafana.annotation;

{
  grafanaDashboards+:: {

    'workload-total.json':

      local newBarplotPanel(graphTitle, graphQuery, graphFormat='Bps', legendFormat='{{namespace}}') =
        local target =
          prometheus.target(
            graphQuery
          ) + {
            intervalFactor: 1,
            legendFormat: legendFormat,
            step: 10,
          };

        graphPanel.new(
          title=graphTitle,
          span=24,
          datasource='$datasource',
          fill=2,
          min_span=24,
          format=graphFormat,
          min=0,
          max=null,
          show_xaxis=false,
          x_axis_mode='series',
          x_axis_values='current',
          lines=false,
          bars=true,
          stack=false,
          legend_show=true,
          legend_values=true,
          legend_min=false,
          legend_max=false,
          legend_current=true,
          legend_avg=false,
          legend_alignAsTable=true,
          legend_rightSide=true,
          legend_sort='current',
          legend_sortDesc=true,
          nullPointMode='null'
        ).addTarget(target) + {
          legend+: {
            hideEmpty: true,
            hideZero: true,
          },
          paceLength: 10,
          tooltip+: {
            sort: 2,
          },
        };

      local newGraphPanel(graphTitle, graphQuery, graphFormat='Bps') =
        local target =
          prometheus.target(
            graphQuery
          ) + {
            intervalFactor: 1,
            legendFormat: '{{pod}}',
            step: 10,
          };

        graphPanel.new(
          title=graphTitle,
          span=12,
          datasource='$datasource',
          fill=2,
          linewidth=2,
          min_span=12,
          format=graphFormat,
          min=0,
          max=null,
          x_axis_mode='time',
          x_axis_values='total',
          lines=true,
          stack=true,
          legend_show=true,
          nullPointMode='connected'
        ).addTarget(target) + {
          legend+: {
            hideEmpty: true,
            hideZero: true,
          },
          paceLength: 10,
          tooltip+: {
            sort: 2,
          },
        };

      local clusterTemplate =
        template.new(
          name='cluster',
          datasource='$datasource',
          query='label_values(kube_pod_info{%(kubeStateMetricsSelector)s}, %(clusterLabel)s)' % $._config,
          hide=if $._config.showMultiCluster then '' else '2',
          refresh=2
        );

      local namespaceTemplate =
        template.new(
          name='namespace',
          datasource='$datasource',
          query='label_values(container_network_receive_packets_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster"}, namespace)' % $._config,
          allValues='.+',
          current='kube-system',
          hide='',
          refresh=2,
          includeAll=true,
          sort=1
        ) + {
          auto: false,
          auto_count: 30,
          auto_min: '10s',
          definition: 'label_values(container_network_receive_packets_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster"}, namespace)' % $._config,
          skipUrlSync: false,
        };

      local workloadTemplate =
        template.new(
          name='workload',
          datasource='$datasource',
          query='label_values(namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster",namespace=~"$namespace"}, workload)' % $._config,
          current='',
          hide='',
          refresh=2,
          includeAll=false,
          sort=1
        ) + {
          auto: false,
          auto_count: 30,
          auto_min: '10s',
          definition: 'label_values(namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster",namespace=~"$namespace"}, workload)' % $._config,
          skipUrlSync: false,
        };

      local typeTemplate =
        template.new(
          name='type',
          datasource='$datasource',
          query='label_values(namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster",namespace=~"$namespace", workload=~"$workload"}, workload_type)' % $._config,
          current='deployment',
          hide='',
          refresh=2,
          includeAll=false,
          sort=0
        ) + {
          auto: false,
          auto_count: 30,
          auto_min: '10s',
          definition: 'label_values(namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster",namespace=~"$namespace", workload=~"$workload"}, workload_type)' % $._config,
          skipUrlSync: false,
        };

      local resolutionTemplate =
        template.new(
          name='resolution',
          datasource='$datasource',
          query='30s,5m,1h',
          current='5m',
          hide='',
          refresh=2,
          includeAll=false,
          sort=1
        ) + {
          auto: false,
          auto_count: 30,
          auto_min: '10s',
          skipUrlSync: false,
          type: 'interval',
          options: [
            {
              selected: false,
              text: '30s',
              value: '30s',
            },
            {
              selected: true,
              text: '5m',
              value: '5m',
            },
            {
              selected: false,
              text: '1h',
              value: '1h',
            },
          ],
        };

      local intervalTemplate =
        template.new(
          name='interval',
          datasource='$datasource',
          query='4h',
          current='5m',
          hide=2,
          refresh=2,
          includeAll=false,
          sort=1
        ) + {
          auto: false,
          auto_count: 30,
          auto_min: '10s',
          skipUrlSync: false,
          type: 'interval',
          options: [
            {
              selected: true,
              text: '4h',
              value: '4h',
            },
          ],
        };

      //#####  Current Bandwidth Row ######

      local currentBandwidthRow =
        row.new(
          title='Current Bandwidth'
        );

      //#####  Average Bandwidth Row ######

      local averageBandwidthRow =
        row.new(
          title='Average Bandwidth',
          collapse=true,
        );

      //#####  Bandwidth History Row ######

      local bandwidthHistoryRow =
        row.new(
          title='Bandwidth HIstory',
        );

      //##### Packet  Row ######
      // collapsed, so row must include panels
      local packetRow =
        row.new(
          title='Packets',
          collapse=true,
        );

      //##### Error Row ######
      // collapsed, so row must include panels
      local errorRow =
        row.new(
          title='Errors',
          collapse=true,
        );

      dashboard.new(
        title='%(dashboardNamePrefix)sNetworking / Workload' % $._config.grafanaK8s,
        tags=($._config.grafanaK8s.dashboardTags),
        editable=true,
        schemaVersion=18,
        refresh=($._config.grafanaK8s.refresh),
        time_from='now-1h',
        time_to='now',
      )
      .addTemplate(
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
      .addTemplate(clusterTemplate)
      .addTemplate(namespaceTemplate)
      .addTemplate(workloadTemplate)
      .addTemplate(typeTemplate)
      .addTemplate(resolutionTemplate)
      .addTemplate(intervalTemplate)
      .addAnnotation(annotation.default)
      .addPanel(currentBandwidthRow, gridPos={ h: 1, w: 24, x: 0, y: 0 })
      .addPanel(
        newBarplotPanel(
          graphTitle='Current Rate of Bytes Received',
          graphQuery=|||
            sort_desc(sum(irate(container_network_receive_bytes_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster",namespace=~"$namespace"}[$interval:$resolution])
            * on (namespace,pod)
            group_left(workload,workload_type) namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster",namespace=~"$namespace", workload=~"$workload", workload_type="$type"}) by (pod))
          ||| % $._config,
          legendFormat='{{ pod }}',
        ),
        gridPos={ h: 9, w: 12, x: 0, y: 1 }
      )
      .addPanel(
        newBarplotPanel(
          graphTitle='Current Rate of Bytes Transmitted',
          graphQuery=|||
            sort_desc(sum(irate(container_network_transmit_bytes_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster",namespace=~"$namespace"}[$interval:$resolution])
            * on (namespace,pod)
            group_left(workload,workload_type) namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster",namespace=~"$namespace", workload=~"$workload", workload_type="$type"}) by (pod))
          ||| % $._config,
          legendFormat='{{ pod }}',
        ),
        gridPos={ h: 9, w: 12, x: 12, y: 1 }
      )
      .addPanel(
        averageBandwidthRow
        .addPanel(
          newBarplotPanel(
            graphTitle='Average Rate of Bytes Received',
            graphQuery=|||
              sort_desc(avg(irate(container_network_receive_bytes_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster",namespace=~"$namespace"}[$interval:$resolution])
              * on (namespace,pod)
              group_left(workload,workload_type) namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster",namespace=~"$namespace", workload=~"$workload", workload_type="$type"}) by (pod))
            ||| % $._config,
            legendFormat='{{ pod }}',
          ),
          gridPos={ h: 9, w: 12, x: 0, y: 11 }
        )
        .addPanel(
          newBarplotPanel(
            graphTitle='Average Rate of Bytes Transmitted',
            graphQuery=|||
              sort_desc(avg(irate(container_network_transmit_bytes_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster",namespace=~"$namespace"}[$interval:$resolution])
              * on (namespace,pod)
              group_left(workload,workload_type) namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster",namespace=~"$namespace", workload=~"$workload", workload_type="$type"}) by (pod))
            ||| % $._config,
            legendFormat='{{ pod }}',
          ),
          gridPos={ h: 9, w: 12, x: 12, y: 11 }
        ),
        gridPos={ h: 1, w: 24, x: 0, y: 10 },
      )
      .addPanel(
        bandwidthHistoryRow, gridPos={ h: 1, w: 24, x: 0, y: 11 }
      )
      .addPanel(
        newGraphPanel(
          graphTitle='Receive Bandwidth',
          graphQuery=|||
            sort_desc(sum(irate(container_network_receive_bytes_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster",namespace=~"$namespace"}[$interval:$resolution])
            * on (namespace,pod)
            group_left(workload,workload_type) namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster",namespace=~"$namespace", workload=~"$workload", workload_type="$type"}) by (pod))
          ||| % $._config,
        ),
        gridPos={ h: 9, w: 12, x: 0, y: 12 }
      )
      .addPanel(
        newGraphPanel(
          graphTitle='Transmit Bandwidth',
          graphQuery=|||
            sort_desc(sum(irate(container_network_transmit_bytes_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster",namespace=~"$namespace"}[$interval:$resolution])
            * on (namespace,pod)
            group_left(workload,workload_type) namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster",namespace=~"$namespace", workload=~"$workload", workload_type="$type"}) by (pod))
          ||| % $._config,
        ),
        gridPos={ h: 9, w: 12, x: 12, y: 12 }
      )
      .addPanel(
        packetRow
        .addPanel(
          newGraphPanel(
            graphTitle='Rate of Received Packets',
            graphQuery=|||
              sort_desc(sum(irate(container_network_receive_packets_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster",namespace=~"$namespace"}[$interval:$resolution])
              * on (namespace,pod)
              group_left(workload,workload_type) namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster",namespace=~"$namespace", workload=~"$workload", workload_type="$type"}) by (pod))
            ||| % $._config,
            graphFormat='pps'
          ),
          gridPos={ h: 9, w: 12, x: 0, y: 22 }
        )
        .addPanel(
          newGraphPanel(
            graphTitle='Rate of Transmitted Packets',
            graphQuery=|||
              sort_desc(sum(irate(container_network_transmit_packets_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster",namespace=~"$namespace"}[$interval:$resolution])
              * on (namespace,pod)
              group_left(workload,workload_type) namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster",namespace=~"$namespace", workload=~"$workload", workload_type="$type"}) by (pod))
            ||| % $._config,
            graphFormat='pps'
          ),
          gridPos={ h: 9, w: 12, x: 12, y: 22 }
        ),
        gridPos={ h: 1, w: 24, x: 0, y: 21 }
      )
      .addPanel(
        errorRow
        .addPanel(
          newGraphPanel(
            graphTitle='Rate of Received Packets Dropped',
            graphQuery=|||
              sort_desc(sum(irate(container_network_receive_packets_dropped_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster",namespace=~"$namespace"}[$interval:$resolution])
              * on (namespace,pod)
              group_left(workload,workload_type) namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster",namespace=~"$namespace", workload=~"$workload", workload_type="$type"}) by (pod))
            ||| % $._config,
            graphFormat='pps'
          ),
          gridPos={ h: 9, w: 12, x: 0, y: 23 }
        )
        .addPanel(
          newGraphPanel(
            graphTitle='Rate of Transmitted Packets Dropped',
            graphQuery=|||
              sort_desc(sum(irate(container_network_transmit_packets_dropped_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster",namespace=~"$namespace"}[$interval:$resolution])
              * on (namespace,pod)
              group_left(workload,workload_type) namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster",namespace=~"$namespace", workload=~"$workload", workload_type="$type"}) by (pod))
            ||| % $._config,
            graphFormat='pps'
          ),
          gridPos={ h: 9, w: 12, x: 12, y: 23 }
        ),
        gridPos={ h: 1, w: 24, x: 0, y: 22 }
      ),
  },
}
