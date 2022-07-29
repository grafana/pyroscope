local grafana = import 'github.com/grafana/grafonnet-lib/grafonnet/grafana.libsonnet';
local g = import 'github.com/grafana/jsonnet-libs/grafana-builder/grafana.libsonnet';
local template = grafana.template;

{
  grafanaDashboards+:: {
    local clusterTemplate =
      template.new(
        name='cluster',
        datasource='$datasource',
        query='label_values(up{%(kubeStateMetricsSelector)s}, %(clusterLabel)s)' % $._config,
        current='',
        hide=if $._config.showMultiCluster then '' else '2',
        refresh=2,
        includeAll=false,
        sort=1
      ),

    local nodeTemplate =
      template.new(
        name='node',
        datasource='$datasource',
        query='label_values(kube_node_info{%(clusterLabel)s="$cluster"}, node)' % $._config,
        current='',
        hide='',
        refresh=2,
        includeAll=false,
        multi=true,
        sort=1
      ),

    'k8s-resources-node.json':
      local tableStyles = {
        pod: {
          alias: 'Pod',
        },
      };

      g.dashboard(
        '%(dashboardNamePrefix)sCompute Resources / Node (Pods)' % $._config.grafanaK8s,
        uid=($._config.grafanaDashboardIDs['k8s-resources-node.json']),
      )
      .addRow(
        g.row('CPU Usage')
        .addPanel(
          g.panel('CPU Usage') +
          g.queryPanel([
            'sum(kube_node_status_capacity{%(clusterLabel)s="$cluster", node=~"$node", resource="cpu"})' % $._config,
            'sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{%(clusterLabel)s="$cluster", node=~"$node"}) by (pod)' % $._config,
          ], [
            'max capacity',
            '{{pod}}',
          ]) +
          g.stack +
          {
            seriesOverrides: [
              {
                alias: 'max capacity',
                color: '#F2495C',
                fill: 0,
                hideTooltip: true,
                legend: true,
                linewidth: 2,
                stack: false,
                hiddenSeries: true,
                dashes: true,
              },
            ],
          },
        )
      )
      .addRow(
        g.row('CPU Quota')
        .addPanel(
          g.panel('CPU Quota') +
          g.tablePanel([
            'sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{%(clusterLabel)s="$cluster", node=~"$node"}) by (pod)' % $._config,
            'sum(cluster:namespace:pod_cpu:active:kube_pod_container_resource_requests{%(clusterLabel)s="$cluster", node=~"$node"}) by (pod)' % $._config,
            'sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{%(clusterLabel)s="$cluster", node=~"$node"}) by (pod) / sum(cluster:namespace:pod_cpu:active:kube_pod_container_resource_requests{%(clusterLabel)s="$cluster", node=~"$node"}) by (pod)' % $._config,
            'sum(cluster:namespace:pod_cpu:active:kube_pod_container_resource_limits{%(clusterLabel)s="$cluster", node=~"$node"}) by (pod)' % $._config,
            'sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{%(clusterLabel)s="$cluster", node=~"$node"}) by (pod) / sum(cluster:namespace:pod_cpu:active:kube_pod_container_resource_limits{%(clusterLabel)s="$cluster", node=~"$node"}) by (pod)' % $._config,
          ], tableStyles {
            'Value #A': { alias: 'CPU Usage' },
            'Value #B': { alias: 'CPU Requests' },
            'Value #C': { alias: 'CPU Requests %', unit: 'percentunit' },
            'Value #D': { alias: 'CPU Limits' },
            'Value #E': { alias: 'CPU Limits %', unit: 'percentunit' },
          })
        )
      )
      .addRow(
        g.row('Memory Usage')
        .addPanel(
          g.panel('Memory Usage (w/o cache)') +
          // Like above, without page cache
          g.queryPanel([
            'sum(kube_node_status_capacity{%(clusterLabel)s="$cluster", node=~"$node", resource="memory"})' % $._config,
            'sum(node_namespace_pod_container:container_memory_working_set_bytes{%(clusterLabel)s="$cluster", node=~"$node", container!=""}) by (pod)' % $._config,
          ], [
            'max capacity',
            '{{pod}}',
          ]) +
          g.stack +
          { yaxes: g.yaxes('bytes') } +
          {
            seriesOverrides: [
              {
                alias: 'max capacity',
                color: '#F2495C',
                fill: 0,
                hideTooltip: true,
                legend: true,
                linewidth: 2,
                stack: false,
                hiddenSeries: true,
                dashes: true,
              },
            ],
          },
        )
      )
      .addRow(
        g.row('Memory Quota')
        .addPanel(
          g.panel('Memory Quota') +
          g.tablePanel([
            'sum(node_namespace_pod_container:container_memory_working_set_bytes{%(clusterLabel)s="$cluster", node=~"$node",container!=""}) by (pod)' % $._config,
            'sum(cluster:namespace:pod_memory:active:kube_pod_container_resource_requests{%(clusterLabel)s="$cluster", node=~"$node"}) by (pod)' % $._config,
            'sum(node_namespace_pod_container:container_memory_working_set_bytes{%(clusterLabel)s="$cluster", node=~"$node",container!=""}) by (pod) / sum(cluster:namespace:pod_memory:active:kube_pod_container_resource_requests{%(clusterLabel)s="$cluster", node=~"$node"}) by (pod)' % $._config,
            'sum(cluster:namespace:pod_memory:active:kube_pod_container_resource_limits{%(clusterLabel)s="$cluster", node=~"$node"}) by (pod)' % $._config,
            'sum(node_namespace_pod_container:container_memory_working_set_bytes{%(clusterLabel)s="$cluster", node=~"$node",container!=""}) by (pod) / sum(cluster:namespace:pod_memory:active:kube_pod_container_resource_limits{%(clusterLabel)s="$cluster", node=~"$node"}) by (pod)' % $._config,
            'sum(node_namespace_pod_container:container_memory_rss{%(clusterLabel)s="$cluster", node=~"$node",container!=""}) by (pod)' % $._config,
            'sum(node_namespace_pod_container:container_memory_cache{%(clusterLabel)s="$cluster", node=~"$node",container!=""}) by (pod)' % $._config,
            'sum(node_namespace_pod_container:container_memory_swap{%(clusterLabel)s="$cluster", node=~"$node",container!=""}) by (pod)' % $._config,
          ], tableStyles {
            'Value #A': { alias: 'Memory Usage', unit: 'bytes' },
            'Value #B': { alias: 'Memory Requests', unit: 'bytes' },
            'Value #C': { alias: 'Memory Requests %', unit: 'percentunit' },
            'Value #D': { alias: 'Memory Limits', unit: 'bytes' },
            'Value #E': { alias: 'Memory Limits %', unit: 'percentunit' },
            'Value #F': { alias: 'Memory Usage (RSS)', unit: 'bytes' },
            'Value #G': { alias: 'Memory Usage (Cache)', unit: 'bytes' },
            'Value #H': { alias: 'Memory Usage (Swap)', unit: 'bytes' },
          })
        )
      ) + {
        templating+: {
          list+: [clusterTemplate, nodeTemplate],
        },
      },
  },
}
