local grafana = import 'github.com/grafana/grafonnet-lib/grafonnet/grafana.libsonnet';
local g = import 'github.com/grafana/jsonnet-libs/grafana-builder/grafana.libsonnet';
local template = grafana.template;

{
  grafanaDashboards+:: {
    local clusterTemplate =
      template.new(
        name='cluster',
        datasource='$datasource',
        query='label_values(up{%(cadvisorSelector)s}, %(clusterLabel)s)' % $._config,
        current='',
        hide=if $._config.showMultiCluster then '' else '2',
        refresh=2,
        includeAll=false,
        sort=1
      ),

    'k8s-resources-cluster.json':
      local tableStyles = {
        namespace: {
          alias: 'Namespace',
          link: '%(prefix)s/d/%(uid)s/k8s-resources-namespace?var-datasource=$datasource&var-cluster=$cluster&var-namespace=$__cell' % { prefix: $._config.grafanaK8s.linkPrefix, uid: std.md5('k8s-resources-namespace.json') },
          linkTooltip: 'Drill down to pods',
        },
        'Value #A': {
          alias: 'Pods',
          linkTooltip: 'Drill down to pods',
          link: '%(prefix)s/d/%(uid)s/k8s-resources-namespace?var-datasource=$datasource&var-cluster=$cluster&var-namespace=$__cell_1' % { prefix: $._config.grafanaK8s.linkPrefix, uid: std.md5('k8s-resources-namespace.json') },
          decimals: 0,
        },
        'Value #B': {
          alias: 'Workloads',
          linkTooltip: 'Drill down to workloads',
          link: '%(prefix)s/d/%(uid)s/k8s-resources-workloads-namespace?var-datasource=$datasource&var-cluster=$cluster&var-namespace=$__cell_1' % { prefix: $._config.grafanaK8s.linkPrefix, uid: std.md5('k8s-resources-workloads-namespace.json') },
          decimals: 0,
        },
      };


      local podWorkloadColumns = [
        'sum(kube_pod_owner{%(kubeStateMetricsSelector)s, %(clusterLabel)s="$cluster"}) by (namespace)' % $._config,
        'count(avg(namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster"}) by (workload, namespace)) by (namespace)' % $._config,
      ];

      local networkColumns = [
        'sum(irate(container_network_receive_bytes_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", %(namespaceLabel)s=~".+"}[%(grafanaIntervalVar)s])) by (namespace)' % $._config,
        'sum(irate(container_network_transmit_bytes_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", %(namespaceLabel)s=~".+"}[%(grafanaIntervalVar)s])) by (namespace)' % $._config,
        'sum(irate(container_network_receive_packets_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", %(namespaceLabel)s=~".+"}[%(grafanaIntervalVar)s])) by (namespace)' % $._config,
        'sum(irate(container_network_transmit_packets_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", %(namespaceLabel)s=~".+"}[%(grafanaIntervalVar)s])) by (namespace)' % $._config,
        'sum(irate(container_network_receive_packets_dropped_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", %(namespaceLabel)s=~".+"}[%(grafanaIntervalVar)s])) by (namespace)' % $._config,
        'sum(irate(container_network_transmit_packets_dropped_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", %(namespaceLabel)s=~".+"}[%(grafanaIntervalVar)s])) by (namespace)' % $._config,
      ];

      local networkTableStyles = {
        namespace: {
          alias: 'Namespace',
          link: '%(prefix)s/d/%(uid)s/k8s-resources-namespace?var-datasource=$datasource&var-cluster=$cluster&var-namespace=$__cell' % { prefix: $._config.grafanaK8s.linkPrefix, uid: std.md5('k8s-resources-namespace.json') },
          linkTooltip: 'Drill down to pods',
        },
        'Value #A': {
          alias: 'Current Receive Bandwidth',
          unit: 'Bps',
        },
        'Value #B': {
          alias: 'Current Transmit Bandwidth',
          unit: 'Bps',
        },
        'Value #C': {
          alias: 'Rate of Received Packets',
          unit: 'pps',
        },
        'Value #D': {
          alias: 'Rate of Transmitted Packets',
          unit: 'pps',
        },
        'Value #E': {
          alias: 'Rate of Received Packets Dropped',
          unit: 'pps',
        },
        'Value #F': {
          alias: 'Rate of Transmitted Packets Dropped',
          unit: 'pps',
        },
      };

      local storageIOColumns = [
        'sum by(namespace) (rate(container_fs_reads_total{%(cadvisorSelector)s, %(diskDeviceSelector)s, %(containerfsSelector)s, %(clusterLabel)s="$cluster", namespace!=""}[%(grafanaIntervalVar)s]))' % $._config,
        'sum by(namespace) (rate(container_fs_writes_total{%(cadvisorSelector)s, %(diskDeviceSelector)s, %(containerfsSelector)s, %(clusterLabel)s="$cluster", namespace!=""}[%(grafanaIntervalVar)s]))' % $._config,
        'sum by(namespace) (rate(container_fs_reads_total{%(cadvisorSelector)s, %(diskDeviceSelector)s, %(containerfsSelector)s, %(clusterLabel)s="$cluster", namespace!=""}[%(grafanaIntervalVar)s]) + rate(container_fs_writes_total{%(cadvisorSelector)s, %(diskDeviceSelector)s, %(containerfsSelector)s, %(clusterLabel)s="$cluster", namespace!=""}[%(grafanaIntervalVar)s]))' % $._config,
        'sum by(namespace) (rate(container_fs_reads_bytes_total{%(cadvisorSelector)s, %(diskDeviceSelector)s, %(containerfsSelector)s, %(clusterLabel)s="$cluster", namespace!=""}[%(grafanaIntervalVar)s]))' % $._config,
        'sum by(namespace) (rate(container_fs_writes_bytes_total{%(cadvisorSelector)s, %(diskDeviceSelector)s, %(containerfsSelector)s, %(clusterLabel)s="$cluster", namespace!=""}[%(grafanaIntervalVar)s]))' % $._config,
        'sum by(namespace) (rate(container_fs_reads_bytes_total{%(cadvisorSelector)s, %(diskDeviceSelector)s, %(containerfsSelector)s, %(clusterLabel)s="$cluster", namespace!=""}[%(grafanaIntervalVar)s]) + rate(container_fs_writes_bytes_total{%(cadvisorSelector)s, %(diskDeviceSelector)s, %(containerfsSelector)s, %(clusterLabel)s="$cluster", namespace!=""}[%(grafanaIntervalVar)s]))' % $._config,
      ];

      local storageIOTableStyles = {
        namespace: {
          alias: 'Namespace',
          link: '%(prefix)s/d/%(uid)s/k8s-resources-namespace?var-datasource=$datasource&var-cluster=$cluster&var-namespace=$__cell' % { prefix: $._config.grafanaK8s.linkPrefix, uid: std.md5('k8s-resources-namespace.json') },
          linkTooltip: 'Drill down to pods',
        },
        'Value #A': {
          alias: 'IOPS(Reads)',
          unit: 'short',
          decimals: -1,
        },
        'Value #B': {
          alias: 'IOPS(Writes)',
          unit: 'short',
          decimals: -1,
        },
        'Value #C': {
          alias: 'IOPS(Reads + Writes)',
          unit: 'short',
          decimals: -1,
        },
        'Value #D': {
          alias: 'Throughput(Read)',
          unit: 'Bps',
        },
        'Value #E': {
          alias: 'Throughput(Write)',
          unit: 'Bps',
        },
        'Value #F': {
          alias: 'Throughput(Read + Write)',
          unit: 'Bps',
        },
      };

      g.dashboard(
        '%(dashboardNamePrefix)sCompute Resources / Cluster' % $._config.grafanaK8s,
        uid=($._config.grafanaDashboardIDs['k8s-resources-cluster.json']),
      )
      .addRow(
        (g.row('Headlines') +
         {
           height: '100px',
           showTitle: false,
         })
        .addPanel(
          g.panel('CPU Utilisation') +
          g.statPanel('cluster:node_cpu:ratio_rate5m{%(clusterLabel)s="$cluster"}' % $._config)
        )
        .addPanel(
          g.panel('CPU Requests Commitment') +
          g.statPanel('sum(namespace_cpu:kube_pod_container_resource_requests:sum{%(clusterLabel)s="$cluster"}) / sum(kube_node_status_allocatable{%(kubeStateMetricsSelector)s,resource="cpu",%(clusterLabel)s="$cluster"})' % $._config)
        )
        .addPanel(
          g.panel('CPU Limits Commitment') +
          g.statPanel('sum(namespace_cpu:kube_pod_container_resource_limits:sum{%(clusterLabel)s="$cluster"}) / sum(kube_node_status_allocatable{%(kubeStateMetricsSelector)s,resource="cpu",%(clusterLabel)s="$cluster"})' % $._config)
        )
        .addPanel(
          g.panel('Memory Utilisation') +
          g.statPanel('1 - sum(:node_memory_MemAvailable_bytes:sum{%(clusterLabel)s="$cluster"}) / sum(node_memory_MemTotal_bytes{%(nodeExporterSelector)s,%(clusterLabel)s="$cluster"})' % $._config)
        )
        .addPanel(
          g.panel('Memory Requests Commitment') +
          g.statPanel('sum(namespace_memory:kube_pod_container_resource_requests:sum{%(clusterLabel)s="$cluster"}) / sum(kube_node_status_allocatable{%(kubeStateMetricsSelector)s,resource="memory",%(clusterLabel)s="$cluster"})' % $._config)
        )
        .addPanel(
          g.panel('Memory Limits Commitment') +
          g.statPanel('sum(namespace_memory:kube_pod_container_resource_limits:sum{%(clusterLabel)s="$cluster"}) / sum(kube_node_status_allocatable{%(kubeStateMetricsSelector)s,resource="memory",%(clusterLabel)s="$cluster"})' % $._config)
        )
      )
      .addRow(
        g.row('CPU')
        .addPanel(
          g.panel('CPU Usage') +
          g.queryPanel('sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{%(clusterLabel)s="$cluster"}) by (namespace)' % $._config, '{{namespace}}') +
          g.stack
        )
      )
      .addRow(
        g.row('CPU Quota')
        .addPanel(
          g.panel('CPU Quota') +
          g.tablePanel(podWorkloadColumns + [
            'sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{%(clusterLabel)s="$cluster"}) by (namespace)' % $._config,
            'sum(namespace_cpu:kube_pod_container_resource_requests:sum{%(clusterLabel)s="$cluster"}) by (namespace)' % $._config,
            'sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{%(clusterLabel)s="$cluster"}) by (namespace) / sum(namespace_cpu:kube_pod_container_resource_requests:sum{%(clusterLabel)s="$cluster"}) by (namespace)' % $._config,
            'sum(namespace_cpu:kube_pod_container_resource_limits:sum{%(clusterLabel)s="$cluster"}) by (namespace)' % $._config,
            'sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{%(clusterLabel)s="$cluster"}) by (namespace) / sum(namespace_cpu:kube_pod_container_resource_limits:sum{%(clusterLabel)s="$cluster"}) by (namespace)' % $._config,
          ], tableStyles {
            'Value #C': { alias: 'CPU Usage' },
            'Value #D': { alias: 'CPU Requests' },
            'Value #E': { alias: 'CPU Requests %', unit: 'percentunit' },
            'Value #F': { alias: 'CPU Limits' },
            'Value #G': { alias: 'CPU Limits %', unit: 'percentunit' },
          })
        )
      )
      .addRow(
        g.row('Memory')
        .addPanel(
          g.panel('Memory Usage (w/o cache)') +
          // Not using container_memory_usage_bytes here because that includes page cache
          g.queryPanel('sum(container_memory_rss{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", container!=""}) by (namespace)' % $._config, '{{namespace}}') +
          g.stack +
          { yaxes: g.yaxes('bytes') },
        )
      )
      .addRow(
        g.row('Memory Requests')
        .addPanel(
          g.panel('Requests by Namespace') +
          g.tablePanel(podWorkloadColumns + [
            // Not using container_memory_usage_bytes here because that includes page cache
            'sum(container_memory_rss{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", container!=""}) by (namespace)' % $._config,
            'sum(namespace_memory:kube_pod_container_resource_requests:sum{%(clusterLabel)s="$cluster"}) by (namespace)' % $._config,
            'sum(container_memory_rss{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", container!=""}) by (namespace) / sum(namespace_memory:kube_pod_container_resource_requests:sum{%(clusterLabel)s="$cluster"}) by (namespace)' % $._config,
            'sum(namespace_memory:kube_pod_container_resource_limits:sum{%(clusterLabel)s="$cluster"}) by (namespace)' % $._config,
            'sum(container_memory_rss{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", container!=""}) by (namespace) / sum(namespace_memory:kube_pod_container_resource_limits:sum{%(clusterLabel)s="$cluster"}) by (namespace)' % $._config,
          ], tableStyles {
            'Value #C': { alias: 'Memory Usage', unit: 'bytes' },
            'Value #D': { alias: 'Memory Requests', unit: 'bytes' },
            'Value #E': { alias: 'Memory Requests %', unit: 'percentunit' },
            'Value #F': { alias: 'Memory Limits', unit: 'bytes' },
            'Value #G': { alias: 'Memory Limits %', unit: 'percentunit' },
          })
        )
      )
      .addRow(
        g.row('Current Network Usage')
        .addPanel(
          g.panel('Current Network Usage') +
          g.tablePanel(
            networkColumns,
            networkTableStyles
          ) +
          { interval: $._config.grafanaK8s.minimumTimeInterval },
        )
      )
      .addRow(
        g.row('Bandwidth')
        .addPanel(
          g.panel('Receive Bandwidth') +
          g.queryPanel('sum(irate(container_network_receive_bytes_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", %(namespaceLabel)s=~".+"}[%(grafanaIntervalVar)s])) by (namespace)' % $._config, '{{namespace}}') +
          g.stack +
          { yaxes: g.yaxes('Bps') },
        )
        .addPanel(
          g.panel('Transmit Bandwidth') +
          g.queryPanel('sum(irate(container_network_transmit_bytes_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", %(namespaceLabel)s=~".+"}[%(grafanaIntervalVar)s])) by (namespace)' % $._config, '{{namespace}}') +
          g.stack +
          { yaxes: g.yaxes('Bps') },
        )
      )
      .addRow(
        g.row('Average Container Bandwidth by Namespace')
        .addPanel(
          g.panel('Average Container Bandwidth by Namespace: Received') +
          g.queryPanel('avg(irate(container_network_receive_bytes_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", %(namespaceLabel)s=~".+"}[%(grafanaIntervalVar)s])) by (namespace)' % $._config, '{{namespace}}') +
          g.stack +
          { yaxes: g.yaxes('Bps') },
        )
        .addPanel(
          g.panel('Average Container Bandwidth by Namespace: Transmitted') +
          g.queryPanel('avg(irate(container_network_transmit_bytes_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", %(namespaceLabel)s=~".+"}[%(grafanaIntervalVar)s])) by (namespace)' % $._config, '{{namespace}}') +
          g.stack +
          { yaxes: g.yaxes('Bps') },
        )
      )
      .addRow(
        g.row('Rate of Packets')
        .addPanel(
          g.panel('Rate of Received Packets') +
          g.queryPanel('sum(irate(container_network_receive_packets_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", %(namespaceLabel)s=~".+"}[%(grafanaIntervalVar)s])) by (namespace)' % $._config, '{{namespace}}') +
          g.stack +
          { yaxes: g.yaxes('pps') },
        )
        .addPanel(
          g.panel('Rate of Transmitted Packets') +
          g.queryPanel('sum(irate(container_network_transmit_packets_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", %(namespaceLabel)s=~".+"}[%(grafanaIntervalVar)s])) by (namespace)' % $._config, '{{namespace}}') +
          g.stack +
          { yaxes: g.yaxes('pps') },
        )
      )
      .addRow(
        g.row('Rate of Packets Dropped')
        .addPanel(
          g.panel('Rate of Received Packets Dropped') +
          g.queryPanel('sum(irate(container_network_receive_packets_dropped_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", %(namespaceLabel)s=~".+"}[%(grafanaIntervalVar)s])) by (namespace)' % $._config, '{{namespace}}') +
          g.stack +
          { yaxes: g.yaxes('pps') },
        )
        .addPanel(
          g.panel('Rate of Transmitted Packets Dropped') +
          g.queryPanel('sum(irate(container_network_transmit_packets_dropped_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", %(namespaceLabel)s=~".+"}[%(grafanaIntervalVar)s])) by (namespace)' % $._config, '{{namespace}}') +
          g.stack +
          { yaxes: g.yaxes('pps') },
        )
      )
      .addRow(
        g.row('Storage IO')
        .addPanel(
          g.panel('IOPS(Reads+Writes)') +
          g.queryPanel('ceil(sum by(namespace) (rate(container_fs_reads_total{%(cadvisorSelector)s, %(containerfsSelector)s, %(diskDeviceSelector)s, %(clusterLabel)s="$cluster", namespace!=""}[%(grafanaIntervalVar)s]) + rate(container_fs_writes_total{%(cadvisorSelector)s, %(containerfsSelector)s, %(clusterLabel)s="$cluster", namespace!=""}[%(grafanaIntervalVar)s])))' % $._config, '{{namespace}}') +
          g.stack +
          { yaxes: g.yaxes('short'), decimals: -1 },

        )
        .addPanel(
          g.panel('ThroughPut(Read+Write)') +
          g.queryPanel('sum by(namespace) (rate(container_fs_reads_bytes_total{%(cadvisorSelector)s, %(containerfsSelector)s, %(diskDeviceSelector)s, %(clusterLabel)s="$cluster", namespace!=""}[%(grafanaIntervalVar)s]) + rate(container_fs_writes_bytes_total{%(cadvisorSelector)s, %(containerfsSelector)s, %(clusterLabel)s="$cluster", namespace!=""}[%(grafanaIntervalVar)s]))' % $._config, '{{namespace}}') +
          g.stack +
          { yaxes: g.yaxes('Bps') },
        )
      )
      .addRow(
        g.row('Storage IO - Distribution')
        .addPanel(
          g.panel('Current Storage IO') +
          g.tablePanel(
            storageIOColumns,
            storageIOTableStyles
          ) +
          {
            sort: {
              col: 4,
              desc: true,
            },
          },
        )
      ) + {
        templating+: {
          list+: [clusterTemplate],
        },
      },
  },
}
