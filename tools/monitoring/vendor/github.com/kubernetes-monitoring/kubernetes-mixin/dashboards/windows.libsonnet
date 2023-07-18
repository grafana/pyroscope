local grafana = import 'github.com/grafana/grafonnet-lib/grafonnet/grafana.libsonnet';
local dashboard = grafana.dashboard;
local prometheus = grafana.prometheus;
local template = grafana.template;
local graphPanel = grafana.graphPanel;
local g = import 'github.com/grafana/jsonnet-libs/grafana-builder/grafana.libsonnet';

{
  grafanaDashboards+:: {
    'k8s-resources-windows-cluster.json':
      local tableStyles = {
        namespace: {
          alias: 'Namespace',
          link: '%(prefix)s/d/%(uid)s/k8s-resources-windows-namespace?var-datasource=$datasource&var-namespace=$__cell' % { prefix: $._config.grafanaK8s.linkPrefix, uid: std.md5('k8s-resources-windows-namespace.json') },
        },
      };

      dashboard.new(
        '%(dashboardNamePrefix)sCompute Resources / Cluster(Windows)' % $._config.grafanaK8s,
        uid=($._config.grafanaDashboardIDs['k8s-resources-windows-cluster.json']),
        tags=($._config.grafanaK8s.dashboardTags),
      ).addTemplate(
        {
          current: {
            text: 'default',
            value: 'default',
          },
          hide: 0,
          label: null,
          name: 'datasource',
          options: [],
          query: 'prometheus',
          refresh: 1,
          regex: $._config.datasourceFilterRegex,
          type: 'datasource',
        },
      ).addTemplate(
        template.new(
          'cluster',
          '$datasource',
          'label_values(up{%(windowsExporterSelector)s}, %(clusterLabel)s)' % $._config,
          label='cluster',
          refresh='time',
          hide=if $._config.showMultiCluster then '' else 'variable',
          sort=1,
        )
      )
      .addRow(
        (g.row('Headlines') +
         {
           height: '100px',
           showTitle: false,
         })
        .addPanel(
          g.panel('CPU Utilisation') +
          g.statPanel('1 - avg(rate(windows_cpu_time_total{%(clusterLabel)s="$cluster", %(windowsExporterSelector)s, mode="idle"}[1m]))' % $._config)
        )
        .addPanel(
          g.panel('CPU Requests Commitment') +
          g.statPanel('sum(kube_pod_windows_container_resource_cpu_cores_request{%(clusterLabel)s="$cluster"}) / sum(node:windows_node_num_cpu:sum{%(clusterLabel)s="$cluster"})' % $._config)
        )
        .addPanel(
          g.panel('CPU Limits Commitment') +
          g.statPanel('sum(kube_pod_windows_container_resource_cpu_cores_limit{%(clusterLabel)s="$cluster"}) / sum(node:windows_node_num_cpu:sum{%(clusterLabel)s="$cluster"})' % $._config)
        )
        .addPanel(
          g.panel('Memory Utilisation') +
          g.statPanel('1 - sum(:windows_node_memory_MemFreeCached_bytes:sum{%(clusterLabel)s="$cluster"}) / sum(:windows_node_memory_MemTotal_bytes:sum{%(clusterLabel)s="$cluster"})' % $._config)
        )
        .addPanel(
          g.panel('Memory Requests Commitment') +
          g.statPanel('sum(kube_pod_windows_container_resource_memory_request{%(clusterLabel)s="$cluster"}) / sum(:windows_node_memory_MemTotal_bytes:sum{%(clusterLabel)s="$cluster"})' % $._config)
        )
        .addPanel(
          g.panel('Memory Limits Commitment') +
          g.statPanel('sum(kube_pod_windows_container_resource_memory_limit{%(clusterLabel)s="$cluster"}) / sum(:windows_node_memory_MemTotal_bytes:sum{%(clusterLabel)s="$cluster"})' % $._config)
        )
      )
      .addRow(
        g.row('CPU')
        .addPanel(
          g.panel('CPU Usage') +
          g.queryPanel('sum(namespace_pod_container:windows_container_cpu_usage_seconds_total:sum_rate{%(clusterLabel)s="$cluster"}) by (namespace)' % $._config, '{{namespace}}') +
          g.stack
        )
      )
      .addRow(
        g.row('CPU Quota')
        .addPanel(
          g.panel('CPU Quota') +
          g.tablePanel([
            'sum(namespace_pod_container:windows_container_cpu_usage_seconds_total:sum_rate{%(clusterLabel)s="$cluster"}) by (namespace)' % $._config,
            'sum(kube_pod_windows_container_resource_cpu_cores_request{%(clusterLabel)s="$cluster"}) by (namespace)' % $._config,
            'sum(namespace_pod_container:windows_container_cpu_usage_seconds_total:sum_rate{%(clusterLabel)s="$cluster"}) by (namespace) / sum(kube_pod_windows_container_resource_cpu_cores_request{%(clusterLabel)s="$cluster"}) by (namespace)' % $._config,
            'sum(kube_pod_windows_container_resource_cpu_cores_limit{%(clusterLabel)s="$cluster"}) by (namespace)' % $._config,
            'sum(namespace_pod_container:windows_container_cpu_usage_seconds_total:sum_rate{%(clusterLabel)s="$cluster"}) by (namespace) / sum(kube_pod_windows_container_resource_cpu_cores_limit{%(clusterLabel)s="$cluster"}) by (namespace)' % $._config,
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
        g.row('Memory')
        .addPanel(
          g.panel('Memory Usage (Private Working Set)') +
          // Not using container_memory_usage_bytes here because that includes page cache
          g.queryPanel('sum(windows_container_private_working_set_usage{%(clusterLabel)s="$cluster"}) by (namespace)' % $._config, '{{namespace}}') +
          g.stack +
          { yaxes: g.yaxes('decbytes') },
        )
      )
      .addRow(
        g.row('Memory Requests')
        .addPanel(
          g.panel('Requests by Namespace') +
          g.tablePanel([
            // Not using container_memory_usage_bytes here because that includes page cache
            'sum(windows_container_private_working_set_usage{%(clusterLabel)s="$cluster"}) by (namespace)' % $._config,
            'sum(kube_pod_windows_container_resource_memory_request{%(clusterLabel)s="$cluster"}) by (namespace)' % $._config,
            'sum(windows_container_private_working_set_usage{%(clusterLabel)s="$cluster"}) by (namespace) / sum(kube_pod_windows_container_resource_memory_request{%(clusterLabel)s="$cluster"}) by (namespace)' % $._config,
            'sum(kube_pod_windows_container_resource_memory_limit{%(clusterLabel)s="$cluster"}) by (namespace)' % $._config,
            'sum(windows_container_private_working_set_usage{%(clusterLabel)s="$cluster"}) by (namespace) / sum(kube_pod_windows_container_resource_memory_limit{%(clusterLabel)s="$cluster"}) by (namespace)' % $._config,
          ], tableStyles {
            'Value #A': { alias: 'Memory Usage', unit: 'decbytes' },
            'Value #B': { alias: 'Memory Requests', unit: 'decbytes' },
            'Value #C': { alias: 'Memory Requests %', unit: 'percentunit' },
            'Value #D': { alias: 'Memory Limits', unit: 'decbytes' },
            'Value #E': { alias: 'Memory Limits %', unit: 'percentunit' },
          })
        )
      ),

    'k8s-resources-windows-namespace.json':
      local tableStyles = {
        pod: {
          alias: 'Pod',
          link: '%(prefix)s/d/%(uid)s/k8s-resources-windows-pod?var-datasource=$datasource&var-namespace=$namespace&var-pod=$__cell' % { prefix: $._config.grafanaK8s.linkPrefix, uid: std.md5('k8s-resources-windows-pod.json') },
        },
      };

      dashboard.new(
        '%(dashboardNamePrefix)sCompute Resources / Namespace(Windows)' % $._config.grafanaK8s,
        uid=($._config.grafanaDashboardIDs['k8s-resources-windows-namespace.json']),
        tags=($._config.grafanaK8s.dashboardTags),
      ).addTemplate(
        {
          current: {
            text: 'default',
            value: $._config.datasourceName,
          },
          hide: 0,
          label: null,
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
          'namespace',
          '$datasource',
          'label_values(windows_pod_container_available, namespace)',
          label='Namespace',
          refresh='time',
          sort=1,
        )
      ).addTemplate(
        template.new(
          'cluster',
          '$datasource',
          'label_values(up{%(windowsExporterSelector)s}, %(clusterLabel)s)' % $._config,
          label='cluster',
          refresh='time',
          hide=if $._config.showMultiCluster then '' else 'variable',
          sort=1,
        )
      )
      .addRow(
        g.row('CPU Usage')
        .addPanel(
          g.panel('CPU Usage') +
          g.queryPanel('sum(namespace_pod_container:windows_container_cpu_usage_seconds_total:sum_rate{%(clusterLabel)s="$cluster", namespace="$namespace"}) by (pod)' % $._config, '{{pod}}') +
          g.stack,
        )
      )
      .addRow(
        g.row('CPU Quota')
        .addPanel(
          g.panel('CPU Quota') +
          g.tablePanel([
            'sum(namespace_pod_container:windows_container_cpu_usage_seconds_total:sum_rate{%(clusterLabel)s="$cluster", namespace="$namespace"}) by (pod)' % $._config,
            'sum(kube_pod_windows_container_resource_cpu_cores_request{%(clusterLabel)s="$cluster", namespace="$namespace"}) by (pod)' % $._config,
            'sum(namespace_pod_container:windows_container_cpu_usage_seconds_total:sum_rate{%(clusterLabel)s="$cluster", namespace="$namespace"}) by (pod) / sum(kube_pod_windows_container_resource_cpu_cores_request{%(clusterLabel)s="$cluster", namespace="$namespace"}) by (pod)' % $._config,
            'sum(kube_pod_windows_container_resource_cpu_cores_limit{%(clusterLabel)s="$cluster", namespace="$namespace"}) by (pod)' % $._config,
            'sum(namespace_pod_container:windows_container_cpu_usage_seconds_total:sum_rate{%(clusterLabel)s="$cluster", namespace="$namespace"}) by (pod) / sum(kube_pod_windows_container_resource_cpu_cores_limit{%(clusterLabel)s="$cluster", namespace="$namespace"}) by (pod)' % $._config,
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
          g.queryPanel('sum(windows_container_private_working_set_usage{%(clusterLabel)s="$cluster", namespace="$namespace"}) by (pod)' % $._config, '{{pod}}') +
          g.stack +
          { yaxes: g.yaxes('decbytes') },
        )
      )
      .addRow(
        g.row('Memory Quota')
        .addPanel(
          g.panel('Memory Quota') +
          g.tablePanel([
            'sum(windows_container_private_working_set_usage{%(clusterLabel)s="$cluster", namespace="$namespace"}) by (pod)' % $._config,
            'sum(kube_pod_windows_container_resource_memory_request{%(clusterLabel)s="$cluster", namespace="$namespace"}) by (pod)' % $._config,
            'sum(windows_container_private_working_set_usage{%(clusterLabel)s="$cluster", namespace="$namespace"}) by (pod) / sum(kube_pod_windows_container_resource_memory_request{%(clusterLabel)s="$cluster", namespace="$namespace"}) by (pod)' % $._config,
            'sum(kube_pod_windows_container_resource_memory_limit{%(clusterLabel)s="$cluster", namespace="$namespace"}) by (pod)' % $._config,
            'sum(windows_container_private_working_set_usage{%(clusterLabel)s="$cluster", namespace="$namespace"}) by (pod) / sum(kube_pod_windows_container_resource_memory_limit{%(clusterLabel)s="$cluster", namespace="$namespace"}) by (pod)' % $._config,
          ], tableStyles {
            'Value #A': { alias: 'Memory Usage', unit: 'decbytes' },
            'Value #B': { alias: 'Memory Requests', unit: 'decbytes' },
            'Value #C': { alias: 'Memory Requests %', unit: 'percentunit' },
            'Value #D': { alias: 'Memory Limits', unit: 'decbytes' },
            'Value #E': { alias: 'Memory Limits %', unit: 'percentunit' },
          })
        )
      ),

    'k8s-resources-windows-pod.json':
      local tableStyles = {
        container: {
          alias: 'Container',
        },
      };

      dashboard.new(
        '%(dashboardNamePrefix)sCompute Resources / Pod(Windows)' % $._config.grafanaK8s,
        uid=($._config.grafanaDashboardIDs['k8s-resources-windows-pod.json']),
        tags=($._config.grafanaK8s.dashboardTags),
      ).addTemplate(
        {
          current: {
            text: 'default',
            value: 'default',
          },
          hide: 0,
          label: null,
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
          'namespace',
          '$datasource',
          'label_values(windows_pod_container_available, namespace)',
          label='Namespace',
          refresh='time',
          sort=1,
        )
      )
      .addTemplate(
        template.new(
          'pod',
          '$datasource',
          'label_values(windows_pod_container_available{namespace="$namespace"}, pod)',
          label='Pod',
          refresh='time',
          sort=1,
        )
      ).addTemplate(
        template.new(
          'cluster',
          '$datasource',
          'label_values(up{%(windowsExporterSelector)s}, %(clusterLabel)s)' % $._config,
          label='cluster',
          refresh='time',
          hide=if $._config.showMultiCluster then '' else 'variable',
          sort=1,
        )
      )
      .addRow(
        g.row('CPU Usage')
        .addPanel(
          g.panel('CPU Usage') +
          g.queryPanel('sum(namespace_pod_container:windows_container_cpu_usage_seconds_total:sum_rate{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod"}) by (container)' % $._config, '{{container}}') +
          g.stack,
        )
      )
      .addRow(
        g.row('CPU Quota')
        .addPanel(
          g.panel('CPU Quota') +
          g.tablePanel([
            'sum(namespace_pod_container:windows_container_cpu_usage_seconds_total:sum_rate{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod"}) by (container)' % $._config,
            'sum(kube_pod_windows_container_resource_cpu_cores_request{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod"}) by (container)' % $._config,
            'sum(namespace_pod_container:windows_container_cpu_usage_seconds_total:sum_rate{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod"}) by (container) / sum(kube_pod_windows_container_resource_cpu_cores_request{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod"}) by (container)' % $._config,
            'sum(kube_pod_windows_container_resource_cpu_cores_limit{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod"}) by (container)' % $._config,
            'sum(namespace_pod_container:windows_container_cpu_usage_seconds_total:sum_rate{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod"}) by (container) / sum(kube_pod_windows_container_resource_cpu_cores_limit{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod"}) by (container)' % $._config,
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
          g.queryPanel('sum(windows_container_private_working_set_usage{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod"}) by (container)' % $._config, '{{container}}') +
          g.stack,
        )
      )
      .addRow(
        g.row('Memory Quota')
        .addPanel(
          g.panel('Memory Quota') +
          g.tablePanel([
            'sum(windows_container_private_working_set_usage{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod"}) by (container)' % $._config,
            'sum(kube_pod_windows_container_resource_memory_request{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod"}) by (container)' % $._config,
            'sum(windows_container_private_working_set_usage{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod"}) by (container) / sum(kube_pod_windows_container_resource_memory_request{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod"}) by (container)' % $._config,
            'sum(kube_pod_windows_container_resource_memory_limit{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod"}) by (container)' % $._config,
            'sum(windows_container_private_working_set_usage{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod"}) by (container) / sum(kube_pod_windows_container_resource_memory_limit{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod"}) by (container)' % $._config,
          ], tableStyles {
            'Value #A': { alias: 'Memory Usage', unit: 'decbytes' },
            'Value #B': { alias: 'Memory Requests', unit: 'decbytes' },
            'Value #C': { alias: 'Memory Requests %', unit: 'percentunit' },
            'Value #D': { alias: 'Memory Limits', unit: 'decbytes' },
            'Value #E': { alias: 'Memory Limits %', unit: 'percentunit' },
          })
        )
      )
      .addRow(
        g.row('Network I/O')
        .addPanel(
          graphPanel.new(
            'Network I/O',
            datasource='$datasource',
            format='bytes',
            min=0,
            legend_rightSide=true,
            legend_alignAsTable=true,
            legend_current=true,
            legend_avg=true,
          )
          .addTarget(prometheus.target(
            'sort_desc(sum by (container) (rate(windows_container_network_received_bytes_total{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod"}[1m])))' % $._config,
            legendFormat='Received : {{ container }}',
          ))
          .addTarget(prometheus.target(
            'sort_desc(sum by (container) (rate(windows_container_network_transmitted_bytes_total{%(clusterLabel)s="$cluster", namespace="$namespace", pod="$pod"}[1m])))' % $._config,
            legendFormat='Transmitted : {{ container }}',
          ))
        )
      ),

    'k8s-windows-cluster-rsrc-use.json':
      local legendLink = '%(prefix)s/d/%(uid)s/k8s-windows-node-rsrc-use' % { prefix: $._config.grafanaK8s.linkPrefix, uid: std.md5('k8s-windows-node-rsrc-use.json') };

      dashboard.new(
        '%(dashboardNamePrefix)sUSE Method / Cluster(Windows)' % $._config.grafanaK8s,
        uid=($._config.grafanaDashboardIDs['k8s-windows-cluster-rsrc-use.json']),
        tags=($._config.grafanaK8s.dashboardTags),
      ).addTemplate(
        {
          current: {
            text: 'default',
            value: 'default',
          },
          hide: 0,
          label: null,
          name: 'datasource',
          options: [],
          query: 'prometheus',
          refresh: 1,
          regex: $._config.datasourceFilterRegex,
          type: 'datasource',
        },
      ).addTemplate(
        template.new(
          'cluster',
          '$datasource',
          'label_values(up{%(windowsExporterSelector)s}, %(clusterLabel)s)' % $._config,
          label='cluster',
          refresh='time',
          hide=if $._config.showMultiCluster then '' else 'variable',
          sort=1,
        )
      )
      .addRow(
        g.row('CPU')
        .addPanel(
          g.panel('CPU Utilisation') +
          g.queryPanel('node:windows_node_cpu_utilisation:avg1m{%(clusterLabel)s="$cluster"} * node:windows_node_num_cpu:sum{%(clusterLabel)s="$cluster"} / scalar(sum(node:windows_node_num_cpu:sum{%(clusterLabel)s="$cluster"}))' % $._config, '{{instance}}', legendLink) +
          g.stack +
          { yaxes: g.yaxes({ format: 'percentunit', max: 1 }) },
        )
      )
      .addRow(
        g.row('Memory')
        .addPanel(
          g.panel('Memory Utilisation') +
          g.queryPanel('node:windows_node_memory_utilisation:ratio{%(clusterLabel)s="$cluster"}' % $._config, '{{instance}}', legendLink) +
          g.stack +
          { yaxes: g.yaxes({ format: 'percentunit', max: 1 }) },
        )
        .addPanel(
          g.panel('Memory Saturation (Swap I/O Pages)') +
          g.queryPanel('node:windows_node_memory_swap_io_pages:irate{%(clusterLabel)s="$cluster"}' % $._config, '{{instance}}', legendLink) +
          g.stack +
          { yaxes: g.yaxes('short') },
        )
      )
      .addRow(
        g.row('Disk')
        .addPanel(
          g.panel('Disk IO Utilisation') +
          // Full utilisation would be all disks on each node spending an average of
          // 1 sec per second doing I/O, normalize by node count for stacked charts
          g.queryPanel('node:windows_node_disk_utilisation:avg_irate{%(clusterLabel)s="$cluster"} / scalar(node:windows_node:sum{%(clusterLabel)s="$cluster"})' % $._config, '{{instance}}', legendLink) +
          g.stack +
          { yaxes: g.yaxes({ format: 'percentunit', max: 1 }) },
        )
      )
      .addRow(
        g.row('Network')
        .addPanel(
          g.panel('Net Utilisation (Transmitted)') +
          g.queryPanel('node:windows_node_net_utilisation:sum_irate{%(clusterLabel)s="$cluster"}' % $._config, '{{instance}}', legendLink) +
          g.stack +
          { yaxes: g.yaxes('Bps') },
        )
        .addPanel(
          g.panel('Net Saturation (Dropped)') +
          g.queryPanel('node:windows_node_net_saturation:sum_irate{%(clusterLabel)s="$cluster"}' % $._config, '{{instance}}', legendLink) +
          g.stack +
          { yaxes: g.yaxes('Bps') },
        )
      )
      .addRow(
        g.row('Storage')
        .addPanel(
          g.panel('Disk Capacity') +
          g.queryPanel(
            |||
              sum by (instance)(node:windows_node_filesystem_usage:{%(clusterLabel)s="$cluster"})
            ||| % $._config, '{{instance}}', legendLink
          ) +
          g.stack +
          { yaxes: g.yaxes({ format: 'percentunit', max: 1 }) },
        ),
      ),

    'k8s-windows-node-rsrc-use.json':
      dashboard.new(
        '%(dashboardNamePrefix)sUSE Method / Node(Windows)' % $._config.grafanaK8s,
        uid=($._config.grafanaDashboardIDs['k8s-windows-node-rsrc-use.json']),
        tags=($._config.grafanaK8s.dashboardTags),
      ).addTemplate(
        {
          current: {
            text: 'default',
            value: 'default',
          },
          hide: 0,
          label: null,
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
          'instance',
          '$datasource',
          'label_values(windows_system_system_up_time, instance)',
          label='Instance',
          refresh='time',
          sort=1,
        )
      ).addTemplate(
        template.new(
          'cluster',
          '$datasource',
          'label_values(up{%(windowsExporterSelector)s}, %(clusterLabel)s)' % $._config,
          label='cluster',
          refresh='time',
          hide=if $._config.showMultiCluster then '' else 'variable',
          sort=1,
        )
      )
      .addRow(
        g.row('CPU')
        .addPanel(
          g.panel('CPU Utilisation') +
          g.queryPanel('node:windows_node_cpu_utilisation:avg1m{%(clusterLabel)s="$cluster", instance="$instance"}' % $._config, 'Utilisation') +
          { yaxes: g.yaxes('percentunit') },
        )
        .addPanel(
          g.panel('CPU Usage Per Core') +
          g.queryPanel('sum by (core) (irate(windows_cpu_time_total{%(clusterLabel)s="$cluster", %(windowsExporterSelector)s, mode!="idle", instance="$instance"}[%(grafanaIntervalVar)s]))' % $._config, '{{core}}') +
          { yaxes: g.yaxes('percentunit') },
        )
      )
      .addRow(
        g.row('Memory')
        .addPanel(
          g.panel('Memory Utilisation %') +
          g.queryPanel('node:windows_node_memory_utilisation:{%(clusterLabel)s="$cluster", instance="$instance"}' % $._config, 'Memory') +
          { yaxes: g.yaxes('percentunit') },
        )
        .addPanel(
          graphPanel.new('Memory Usage',
                         datasource='$datasource',
                         format='bytes',)
          .addTarget(prometheus.target(
            |||
              max(
                windows_os_visible_memory_bytes{%(clusterLabel)s="$cluster", %(windowsExporterSelector)s, instance="$instance"}
                - windows_memory_available_bytes{%(clusterLabel)s="$cluster", %(windowsExporterSelector)s, instance="$instance"}
              )
            ||| % $._config, legendFormat='memory used'
          ))
          .addTarget(prometheus.target('max(node:windows_node_memory_totalCached_bytes:sum{%(clusterLabel)s="$cluster", instance="$instance"})' % $._config, legendFormat='memory cached'))
          .addTarget(prometheus.target('max(windows_memory_available_bytes{%(clusterLabel)s="$cluster", %(windowsExporterSelector)s, instance="$instance"})' % $._config, legendFormat='memory free'))
        )
        .addPanel(
          g.panel('Memory Saturation (Swap I/O) Pages') +
          g.queryPanel('node:windows_node_memory_swap_io_pages:irate{%(clusterLabel)s="$cluster", instance="$instance"}' % $._config, 'Swap IO') +
          { yaxes: g.yaxes('short') },
        )
      )
      .addRow(
        g.row('Disk')
        .addPanel(
          g.panel('Disk IO Utilisation') +
          g.queryPanel('node:windows_node_disk_utilisation:avg_irate{%(clusterLabel)s="$cluster", instance="$instance"}' % $._config, 'Utilisation') +
          { yaxes: g.yaxes('percentunit') },
        )
        .addPanel(
          graphPanel.new('Disk I/O', datasource='$datasource')
          .addTarget(prometheus.target('max(rate(windows_logical_disk_read_bytes_total{%(clusterLabel)s="$cluster", %(windowsExporterSelector)s, instance="$instance"}[2m]))' % $._config, legendFormat='read'))
          .addTarget(prometheus.target('max(rate(windows_logical_disk_write_bytes_total{%(clusterLabel)s="$cluster", %(windowsExporterSelector)s, instance="$instance"}[2m]))' % $._config, legendFormat='written'))
          .addTarget(prometheus.target('max(rate(windows_logical_disk_read_seconds_total{%(clusterLabel)s="$cluster", %(windowsExporterSelector)s,  instance="$instance"}[2m]) + rate(windows_logical_disk_write_seconds_total{%(clusterLabel)s="$cluster", %(windowsExporterSelector)s, instance="$instance"}[2m]))' % $._config, legendFormat='io time')) +
          {
            seriesOverrides: [
              {
                alias: 'read',
                yaxis: 1,
              },
              {
                alias: 'io time',
                yaxis: 2,
              },
            ],
            yaxes: [
              self.yaxe(format='bytes'),
              self.yaxe(format='ms'),
            ],
          }
        )
      )
      .addRow(
        g.row('Net')
        .addPanel(
          g.panel('Net Utilisation (Transmitted)') +
          g.queryPanel('node:windows_node_net_utilisation:sum_irate{%(clusterLabel)s="$cluster", instance="$instance"}' % $._config, 'Utilisation') +
          { yaxes: g.yaxes('Bps') },
        )
        .addPanel(
          g.panel('Net Saturation (Dropped)') +
          g.queryPanel('node:windows_node_net_saturation:sum_irate{%(clusterLabel)s="$cluster", instance="$instance"}' % $._config, 'Saturation') +
          { yaxes: g.yaxes('Bps') },
        )
      )
      .addRow(
        g.row('Disk')
        .addPanel(
          g.panel('Disk Utilisation') +
          g.queryPanel(
            |||
              node:windows_node_filesystem_usage:{%(clusterLabel)s="$cluster", instance="$instance"}
            ||| % $._config,
            '{{volume}}',
          ) +
          { yaxes: g.yaxes('percentunit') },
        ),
      ),
  },
}
