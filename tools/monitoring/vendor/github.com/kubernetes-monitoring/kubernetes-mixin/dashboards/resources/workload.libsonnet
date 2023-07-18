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

    local namespaceTemplate =
      template.new(
        name='namespace',
        datasource='$datasource',
        query='label_values(kube_namespace_status_phase{%(kubeStateMetricsSelector)s, %(clusterLabel)s="$cluster"}, namespace)' % $._config,
        current='',
        hide='',
        refresh=2,
        includeAll=false,
        multi=false,
        sort=1
      ),

    local workloadTypeTemplate =
      template.new(
        name='type',
        datasource='$datasource',
        query='label_values(namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster", namespace="$namespace"}, workload_type)' % $._config.clusterLabel,
        current='',
        hide='',
        refresh=2,
        includeAll=false,
        sort=1
      ),

    local workloadTemplate =
      template.new(
        name='workload',
        datasource='$datasource',
        query='label_values(namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster", namespace="$namespace", workload_type="$type"}, workload)' % $._config.clusterLabel,
        current='',
        hide='',
        refresh=2,
        includeAll=false,
        sort=1
      ),
    'k8s-resources-workload.json':
      local tableStyles = {
        pod: {
          alias: 'Pod',
          link: '%(prefix)s/d/%(uid)s/k8s-resources-pod?var-datasource=$datasource&var-cluster=$cluster&var-namespace=$namespace&var-pod=$__cell' % { prefix: $._config.grafanaK8s.linkPrefix, uid: std.md5('k8s-resources-pod.json') },
        },
      };

      local networkColumns = [
        |||
          (sum(irate(container_network_receive_bytes_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", %(namespaceLabel)s="$namespace"}[%(grafanaIntervalVar)s])
          * on (namespace,pod)
          group_left(workload,workload_type) namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster", %(namespaceLabel)s="$namespace", workload=~"$workload", workload_type="$type"}) by (pod))
        ||| % $._config,
        |||
          (sum(irate(container_network_transmit_bytes_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", %(namespaceLabel)s="$namespace"}[%(grafanaIntervalVar)s])
          * on (namespace,pod)
          group_left(workload,workload_type) namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster", %(namespaceLabel)s="$namespace", workload=~"$workload", workload_type="$type"}) by (pod))
        ||| % $._config,
        |||
          (sum(irate(container_network_receive_packets_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", %(namespaceLabel)s="$namespace"}[%(grafanaIntervalVar)s])
          * on (namespace,pod)
          group_left(workload,workload_type) namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster", %(namespaceLabel)s="$namespace", workload=~"$workload", workload_type="$type"}) by (pod))
        ||| % $._config,
        |||
          (sum(irate(container_network_transmit_packets_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", %(namespaceLabel)s="$namespace"}[%(grafanaIntervalVar)s])
          * on (namespace,pod)
          group_left(workload,workload_type) namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster", %(namespaceLabel)s="$namespace", workload=~"$workload", workload_type="$type"}) by (pod))
        ||| % $._config,
        |||
          (sum(irate(container_network_receive_packets_dropped_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", %(namespaceLabel)s="$namespace"}[%(grafanaIntervalVar)s])
          * on (namespace,pod)
          group_left(workload,workload_type) namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster", %(namespaceLabel)s="$namespace", workload=~"$workload", workload_type="$type"}) by (pod))
        ||| % $._config,
        |||
          (sum(irate(container_network_transmit_packets_dropped_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", %(namespaceLabel)s="$namespace"}[%(grafanaIntervalVar)s])
          * on (namespace,pod)
          group_left(workload,workload_type) namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster", %(namespaceLabel)s="$namespace", workload=~"$workload", workload_type="$type"}) by (pod))
        ||| % $._config,
      ];

      local networkTableStyles = {
        pod: {
          alias: 'Pod',
          link: '%(prefix)s/d/%(uid)s/k8s-resources-pod?var-datasource=$datasource&var-cluster=$cluster&var-namespace=$namespace&var-pod=$__cell' % { prefix: $._config.grafanaK8s.linkPrefix, uid: std.md5('k8s-resources-pod.json') },
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


      local cpuUsageQuery = |||
        sum(
            node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{%(clusterLabel)s="$cluster", namespace="$namespace"}
          * on(namespace,pod)
            group_left(workload, workload_type) namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster", namespace="$namespace", workload="$workload", workload_type="$type"}
        ) by (pod)
      ||| % $._config;

      local cpuRequestsQuery = |||
        sum(
            kube_pod_container_resource_requests{%(kubeStateMetricsSelector)s, %(clusterLabel)s="$cluster", namespace="$namespace", resource="cpu"}
          * on(namespace,pod)
            group_left(workload, workload_type) namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster", namespace="$namespace", workload="$workload", workload_type="$type"}
        ) by (pod)
      ||| % $._config;

      local cpuLimitsQuery = std.strReplace(cpuRequestsQuery, 'requests', 'limits');

      local memUsageQuery = |||
        sum(
            container_memory_working_set_bytes{%(clusterLabel)s="$cluster", namespace="$namespace", container!="", image!=""}
          * on(namespace,pod)
            group_left(workload, workload_type) namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster", namespace="$namespace", workload="$workload", workload_type="$type"}
        ) by (pod)
      ||| % $._config;
      local memRequestsQuery = std.strReplace(cpuRequestsQuery, 'cpu', 'memory');
      local memLimitsQuery = std.strReplace(cpuLimitsQuery, 'cpu', 'memory');

      g.dashboard(
        '%(dashboardNamePrefix)sCompute Resources / Workload' % $._config.grafanaK8s,
        uid=($._config.grafanaDashboardIDs['k8s-resources-workload.json']),
      )
      .addRow(
        g.row('CPU Usage')
        .addPanel(
          g.panel('CPU Usage') +
          g.queryPanel(cpuUsageQuery, '{{pod}}') +
          g.stack,
        )
      )
      .addRow(
        g.row('CPU Quota')
        .addPanel(
          g.panel('CPU Quota') +
          g.tablePanel([
            cpuUsageQuery,
            cpuRequestsQuery,
            cpuUsageQuery + '/' + cpuRequestsQuery,
            cpuLimitsQuery,
            cpuUsageQuery + '/' + cpuLimitsQuery,
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
          g.panel('Memory Usage') +
          g.queryPanel(memUsageQuery, '{{pod}}') +
          g.stack +
          { yaxes: g.yaxes('bytes') },
        )
      )
      .addRow(
        g.row('Memory Quota')
        .addPanel(
          g.panel('Memory Quota') +
          g.tablePanel([
            memUsageQuery,
            memRequestsQuery,
            memUsageQuery + '/' + memRequestsQuery,
            memLimitsQuery,
            memUsageQuery + '/' + memLimitsQuery,
          ], tableStyles {
            'Value #A': { alias: 'Memory Usage', unit: 'bytes' },
            'Value #B': { alias: 'Memory Requests', unit: 'bytes' },
            'Value #C': { alias: 'Memory Requests %', unit: 'percentunit' },
            'Value #D': { alias: 'Memory Limits', unit: 'bytes' },
            'Value #E': { alias: 'Memory Limits %', unit: 'percentunit' },
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
          ),
        )
      )
      .addRow(
        g.row('Bandwidth')
        .addPanel(
          g.panel('Receive Bandwidth') +
          g.queryPanel(|||
            (sum(irate(container_network_receive_bytes_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", namespace="$namespace"}[%(grafanaIntervalVar)s])
            * on (namespace,pod)
            group_left(workload,workload_type) namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster", %(namespaceLabel)s="$namespace", workload=~"$workload", workload_type="$type"}) by (pod))
          ||| % $._config, '{{pod}}') +
          g.stack +
          { yaxes: g.yaxes('Bps') },
        )
        .addPanel(
          g.panel('Transmit Bandwidth') +
          g.queryPanel(|||
            (sum(irate(container_network_transmit_bytes_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", namespace="$namespace"}[%(grafanaIntervalVar)s])
            * on (namespace,pod)
            group_left(workload,workload_type) namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster", %(namespaceLabel)s="$namespace", workload=~"$workload", workload_type="$type"}) by (pod))
          ||| % $._config, '{{pod}}') +
          g.stack +
          { yaxes: g.yaxes('Bps') },
        )
      )
      .addRow(
        g.row('Average Container Bandwidth by Pod')
        .addPanel(
          g.panel('Average Container Bandwidth by Pod: Received') +
          g.queryPanel(|||
            (avg(irate(container_network_receive_bytes_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", namespace="$namespace"}[%(grafanaIntervalVar)s])
            * on (namespace,pod)
            group_left(workload,workload_type) namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster", %(namespaceLabel)s="$namespace", workload=~"$workload", workload_type="$type"}) by (pod))
          ||| % $._config, '{{pod}}') +
          g.stack +
          { yaxes: g.yaxes('Bps') },
        )
        .addPanel(
          g.panel('Average Container Bandwidth by Pod: Transmitted') +
          g.queryPanel(|||
            (avg(irate(container_network_transmit_bytes_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", namespace="$namespace"}[%(grafanaIntervalVar)s])
            * on (namespace,pod)
            group_left(workload,workload_type) namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster", %(namespaceLabel)s="$namespace", workload=~"$workload", workload_type="$type"}) by (pod))
          ||| % $._config, '{{pod}}') +
          g.stack +
          { yaxes: g.yaxes('Bps') },
        )
      )
      .addRow(
        g.row('Rate of Packets')
        .addPanel(
          g.panel('Rate of Received Packets') +
          g.queryPanel(|||
            (sum(irate(container_network_receive_packets_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", namespace="$namespace"}[%(grafanaIntervalVar)s])
            * on (namespace,pod)
            group_left(workload,workload_type) namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster", %(namespaceLabel)s="$namespace", workload=~"$workload", workload_type="$type"}) by (pod))
          ||| % $._config, '{{pod}}') +
          g.stack +
          { yaxes: g.yaxes('pps') },
        )
        .addPanel(
          g.panel('Rate of Transmitted Packets') +
          g.queryPanel(|||
            (sum(irate(container_network_transmit_packets_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", namespace="$namespace"}[%(grafanaIntervalVar)s])
            * on (namespace,pod)
            group_left(workload,workload_type) namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster", %(namespaceLabel)s="$namespace", workload=~"$workload", workload_type="$type"}) by (pod))
          ||| % $._config, '{{pod}}') +
          g.stack +
          { yaxes: g.yaxes('pps') },
        )
      )
      .addRow(
        g.row('Rate of Packets Dropped')
        .addPanel(
          g.panel('Rate of Received Packets Dropped') +
          g.queryPanel(|||
            (sum(irate(container_network_receive_packets_dropped_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", namespace="$namespace"}[%(grafanaIntervalVar)s])
            * on (namespace,pod)
            group_left(workload,workload_type) namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster", %(namespaceLabel)s="$namespace", workload=~"$workload", workload_type="$type"}) by (pod))
          ||| % $._config, '{{pod}}') +
          g.stack +
          { yaxes: g.yaxes('pps') },
        )
        .addPanel(
          g.panel('Rate of Transmitted Packets Dropped') +
          g.queryPanel(|||
            (sum(irate(container_network_transmit_packets_dropped_total{%(cadvisorSelector)s, %(clusterLabel)s="$cluster", namespace="$namespace"}[%(grafanaIntervalVar)s])
            * on (namespace,pod)
            group_left(workload,workload_type) namespace_workload_pod:kube_pod_owner:relabel{%(clusterLabel)s="$cluster", %(namespaceLabel)s="$namespace", workload=~"$workload", workload_type="$type"}) by (pod))
          ||| % $._config, '{{pod}}') +
          g.stack +
          { yaxes: g.yaxes('pps') },
        )
      ) + {
        templating+: {
          list+: [clusterTemplate, namespaceTemplate, workloadTypeTemplate, workloadTemplate],
        },
      },
  },
}
