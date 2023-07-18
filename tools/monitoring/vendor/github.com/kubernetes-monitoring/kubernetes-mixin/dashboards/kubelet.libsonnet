local grafana = import 'github.com/grafana/grafonnet-lib/grafonnet/grafana.libsonnet';
local dashboard = grafana.dashboard;
local prometheus = grafana.prometheus;
local template = grafana.template;
local graphPanel = grafana.graphPanel;
local statPanel = grafana.statPanel;

{
  grafanaDashboards+:: {
    'kubelet.json':
      local upCount =
        statPanel.new(
          'Running Kubelets',
          datasource='$datasource',
          reducerFunction='lastNotNull',
        )
        .addTarget(prometheus.target('sum(kubelet_node_name{%(clusterLabel)s="$cluster", %(kubeletSelector)s})' % $._config));

      local runningPodCount =
        statPanel.new(
          'Running Pods',
          datasource='$datasource',
          reducerFunction='lastNotNull',
        )
        // TODO: The second query selected by the OR operator is for backward compatibility with kubernetes < 1.19, so this can be retored to a single query once 1.23 is out
        .addTarget(prometheus.target('sum(kubelet_running_pods{%(clusterLabel)s="$cluster", %(kubeletSelector)s, instance=~"$instance"}) OR sum(kubelet_running_pod_count{%(clusterLabel)s="$cluster", %(kubeletSelector)s, instance=~"$instance"})' % $._config, legendFormat='{{instance}}'));

      local runningContainerCount =
        statPanel.new(
          'Running Containers',
          datasource='$datasource',
          reducerFunction='lastNotNull',
        )
        // TODO: The second query selected by the OR operator is for backward compatibility with kubernetes < 1.19, so this can be retored to a single query once 1.23 is out
        .addTarget(prometheus.target('sum(kubelet_running_containers{%(clusterLabel)s="$cluster", %(kubeletSelector)s, instance=~"$instance"}) OR sum(kubelet_running_container_count{%(clusterLabel)s="$cluster", %(kubeletSelector)s, instance=~"$instance"})' % $._config, legendFormat='{{instance}}'));

      local actualVolumeCount =
        statPanel.new(
          'Actual Volume Count',
          datasource='$datasource',
          reducerFunction='lastNotNull',
        )
        .addTarget(prometheus.target('sum(volume_manager_total_volumes{%(clusterLabel)s="$cluster", %(kubeletSelector)s, instance=~"$instance", state="actual_state_of_world"})' % $._config, legendFormat='{{instance}}'));

      local desiredVolumeCount =
        statPanel.new(
          'Desired Volume Count',
          datasource='$datasource',
          reducerFunction='lastNotNull',
        )
        .addTarget(prometheus.target('sum(volume_manager_total_volumes{%(clusterLabel)s="$cluster", %(kubeletSelector)s, instance=~"$instance",state="desired_state_of_world"})' % $._config, legendFormat='{{instance}}'));

      local configErrorCount =
        statPanel.new(
          'Config Error Count',
          datasource='$datasource',
          reducerFunction='lastNotNull',
        )
        .addTarget(prometheus.target('sum(rate(kubelet_node_config_error{%(clusterLabel)s="$cluster", %(kubeletSelector)s, instance=~"$instance"}[%(grafanaIntervalVar)s]))' % $._config, legendFormat='{{instance}}'));

      local operationRate =
        graphPanel.new(
          'Operation Rate',
          datasource='$datasource',
          format='ops',
          legend_show=true,
          legend_values=true,
          legend_current=true,
          legend_alignAsTable=true,
          legend_rightSide=true,
        )
        .addTarget(prometheus.target('sum(rate(kubelet_runtime_operations_total{%(clusterLabel)s="$cluster",%(kubeletSelector)s,instance=~"$instance"}[%(grafanaIntervalVar)s])) by (operation_type, instance)' % $._config, legendFormat='{{instance}} {{operation_type}}'));

      local operationErrorRate =
        graphPanel.new(
          'Operation Error Rate',
          datasource='$datasource',
          format='ops',
          legend_show=true,
          legend_values=true,
          legend_current=true,
          legend_alignAsTable=true,
          legend_rightSide=true,
        )
        .addTarget(prometheus.target('sum(rate(kubelet_runtime_operations_errors_total{%(clusterLabel)s="$cluster",%(kubeletSelector)s,instance=~"$instance"}[%(grafanaIntervalVar)s])) by (instance, operation_type)' % $._config, legendFormat='{{instance}} {{operation_type}}'));

      local operationLatency =
        graphPanel.new(
          'Operation duration 99th quantile',
          datasource='$datasource',
          format='s',
          legend_show=true,
          legend_values=true,
          legend_current=true,
          legend_alignAsTable=true,
          legend_rightSide=true,
        )
        .addTarget(prometheus.target('histogram_quantile(0.99, sum(rate(kubelet_runtime_operations_duration_seconds_bucket{%(clusterLabel)s="$cluster",%(kubeletSelector)s,instance=~"$instance"}[%(grafanaIntervalVar)s])) by (instance, operation_type, le))' % $._config, legendFormat='{{instance}} {{operation_type}}'));

      local podStartRate =
        graphPanel.new(
          'Pod Start Rate',
          datasource='$datasource',
          format='ops',
          legend_show=true,
          legend_values=true,
          legend_current=true,
          legend_alignAsTable=true,
          legend_rightSide=true,
        )
        .addTarget(prometheus.target('sum(rate(kubelet_pod_start_duration_seconds_count{%(clusterLabel)s="$cluster",%(kubeletSelector)s,instance=~"$instance"}[%(grafanaIntervalVar)s])) by (instance)' % $._config, legendFormat='{{instance}} pod'))
        .addTarget(prometheus.target('sum(rate(kubelet_pod_worker_duration_seconds_count{%(clusterLabel)s="$cluster",%(kubeletSelector)s,instance=~"$instance"}[%(grafanaIntervalVar)s])) by (instance)' % $._config, legendFormat='{{instance}} worker'));

      local podStartLatency =
        graphPanel.new(
          'Pod Start Duration',
          datasource='$datasource',
          format='s',
          legend_show=true,
          legend_values=true,
          legend_current=true,
          legend_alignAsTable=true,
          legend_rightSide=true,
        )
        .addTarget(prometheus.target('histogram_quantile(0.99, sum(rate(kubelet_pod_start_duration_seconds_count{%(clusterLabel)s="$cluster",%(kubeletSelector)s,instance=~"$instance"}[%(grafanaIntervalVar)s])) by (instance, le))' % $._config, legendFormat='{{instance}} pod'))
        .addTarget(prometheus.target('histogram_quantile(0.99, sum(rate(kubelet_pod_worker_duration_seconds_bucket{%(clusterLabel)s="$cluster",%(kubeletSelector)s,instance=~"$instance"}[%(grafanaIntervalVar)s])) by (instance, le))' % $._config, legendFormat='{{instance}} worker'));

      local storageOperationRate =
        graphPanel.new(
          'Storage Operation Rate',
          datasource='$datasource',
          format='ops',
          legend_show=true,
          legend_values=true,
          legend_current=true,
          legend_alignAsTable=true,
          legend_rightSide=true,
          legend_hideEmpty=true,
          legend_hideZero=true,
        )
        .addTarget(prometheus.target('sum(rate(storage_operation_duration_seconds_count{%(clusterLabel)s="$cluster",%(kubeletSelector)s,instance=~"$instance"}[%(grafanaIntervalVar)s])) by (instance, operation_name, volume_plugin)' % $._config, legendFormat='{{instance}} {{operation_name}} {{volume_plugin}}'));

      local storageOperationErrorRate =
        graphPanel.new(
          'Storage Operation Error Rate',
          datasource='$datasource',
          format='ops',
          legend_show=true,
          legend_values=true,
          legend_current=true,
          legend_alignAsTable=true,
          legend_rightSide=true,
          legend_hideEmpty=true,
          legend_hideZero=true,
        )
        .addTarget(prometheus.target('sum(rate(storage_operation_errors_total{%(clusterLabel)s="$cluster",%(kubeletSelector)s,instance=~"$instance"}[%(grafanaIntervalVar)s])) by (instance, operation_name, volume_plugin)' % $._config, legendFormat='{{instance}} {{operation_name}} {{volume_plugin}}'));


      local storageOperationLatency =
        graphPanel.new(
          'Storage Operation Duration 99th quantile',
          datasource='$datasource',
          format='s',
          legend_values=true,
          legend_current=true,
          legend_alignAsTable=true,
          legend_rightSide=true,
          legend_hideEmpty=true,
          legend_hideZero=true,
        )
        .addTarget(prometheus.target('histogram_quantile(0.99, sum(rate(storage_operation_duration_seconds_bucket{%(clusterLabel)s="$cluster", %(kubeletSelector)s, instance=~"$instance"}[%(grafanaIntervalVar)s])) by (instance, operation_name, volume_plugin, le))' % $._config, legendFormat='{{instance}} {{operation_name}} {{volume_plugin}}'));

      local cgroupManagerRate =
        graphPanel.new(
          'Cgroup manager operation rate',
          datasource='$datasource',
          format='ops',
          legend_show=true,
          legend_values=true,
          legend_current=true,
          legend_alignAsTable=true,
          legend_rightSide=true,
        )
        .addTarget(prometheus.target('sum(rate(kubelet_cgroup_manager_duration_seconds_count{%(clusterLabel)s="$cluster", %(kubeletSelector)s, instance=~"$instance"}[%(grafanaIntervalVar)s])) by (instance, operation_type)' % $._config, legendFormat='{{operation_type}}'));

      local cgroupManagerDuration =
        graphPanel.new(
          'Cgroup manager 99th quantile',
          datasource='$datasource',
          format='s',
          legend_show=true,
          legend_values=true,
          legend_current=true,
          legend_alignAsTable=true,
          legend_rightSide=true,
        )
        .addTarget(prometheus.target('histogram_quantile(0.99, sum(rate(kubelet_cgroup_manager_duration_seconds_bucket{%(clusterLabel)s="$cluster", %(kubeletSelector)s, instance=~"$instance"}[%(grafanaIntervalVar)s])) by (instance, operation_type, le))' % $._config, legendFormat='{{instance}} {{operation_type}}'));

      local plegRelistRate =
        graphPanel.new(
          'PLEG relist rate',
          datasource='$datasource',
          description='Pod lifecycle event generator',
          format='ops',
          legend_show=true,
          legend_values=true,
          legend_current=true,
          legend_alignAsTable=true,
          legend_rightSide=true,
        )
        .addTarget(prometheus.target('sum(rate(kubelet_pleg_relist_duration_seconds_count{%(clusterLabel)s="$cluster", %(kubeletSelector)s, instance=~"$instance"}[%(grafanaIntervalVar)s])) by (instance)' % $._config, legendFormat='{{instance}}'));

      local plegRelistDuration =
        graphPanel.new(
          'PLEG relist duration',
          datasource='$datasource',
          format='s',
          legend_show=true,
          legend_values=true,
          legend_current=true,
          legend_alignAsTable=true,
          legend_rightSide=true,
        )
        .addTarget(prometheus.target('histogram_quantile(0.99, sum(rate(kubelet_pleg_relist_duration_seconds_bucket{%(clusterLabel)s="$cluster",%(kubeletSelector)s,instance=~"$instance"}[%(grafanaIntervalVar)s])) by (instance, le))' % $._config, legendFormat='{{instance}}'));

      local plegRelistInterval =
        graphPanel.new(
          'PLEG relist interval',
          datasource='$datasource',
          format='s',
          legend_show=true,
          legend_values=true,
          legend_current=true,
          legend_alignAsTable=true,
          legend_rightSide=true,
        )
        .addTarget(prometheus.target('histogram_quantile(0.99, sum(rate(kubelet_pleg_relist_interval_seconds_bucket{%(clusterLabel)s="$cluster",%(kubeletSelector)s,instance=~"$instance"}[%(grafanaIntervalVar)s])) by (instance, le))' % $._config, legendFormat='{{instance}}'));

      local rpcRate =
        graphPanel.new(
          'RPC Rate',
          datasource='$datasource',
          format='ops',
        )
        .addTarget(prometheus.target('sum(rate(rest_client_requests_total{%(clusterLabel)s="$cluster",%(kubeletSelector)s, instance=~"$instance",code=~"2.."}[%(grafanaIntervalVar)s]))' % $._config, legendFormat='2xx'))
        .addTarget(prometheus.target('sum(rate(rest_client_requests_total{%(clusterLabel)s="$cluster",%(kubeletSelector)s, instance=~"$instance",code=~"3.."}[%(grafanaIntervalVar)s]))' % $._config, legendFormat='3xx'))
        .addTarget(prometheus.target('sum(rate(rest_client_requests_total{%(clusterLabel)s="$cluster",%(kubeletSelector)s, instance=~"$instance",code=~"4.."}[%(grafanaIntervalVar)s]))' % $._config, legendFormat='4xx'))
        .addTarget(prometheus.target('sum(rate(rest_client_requests_total{%(clusterLabel)s="$cluster",%(kubeletSelector)s, instance=~"$instance",code=~"5.."}[%(grafanaIntervalVar)s]))' % $._config, legendFormat='5xx'));

      local requestDuration =
        graphPanel.new(
          'Request duration 99th quantile',
          datasource='$datasource',
          format='s',
          legend_show=true,
          legend_values=true,
          legend_current=true,
          legend_alignAsTable=true,
          legend_rightSide=true,
        )
        .addTarget(prometheus.target('histogram_quantile(0.99, sum(rate(rest_client_request_duration_seconds_bucket{%(clusterLabel)s="$cluster",%(kubeletSelector)s, instance=~"$instance"}[%(grafanaIntervalVar)s])) by (instance, verb, url, le))' % $._config, legendFormat='{{instance}} {{verb}} {{url}}'));

      local memory =
        graphPanel.new(
          'Memory',
          datasource='$datasource',
          format='bytes',
        )
        .addTarget(prometheus.target('process_resident_memory_bytes{%(clusterLabel)s="$cluster",%(kubeletSelector)s,instance=~"$instance"}' % $._config, legendFormat='{{instance}}'));

      local cpu =
        graphPanel.new(
          'CPU usage',
          datasource='$datasource',
          format='short',
        )
        .addTarget(prometheus.target('rate(process_cpu_seconds_total{%(clusterLabel)s="$cluster",%(kubeletSelector)s,instance=~"$instance"}[%(grafanaIntervalVar)s])' % $._config, legendFormat='{{instance}}'));

      local goroutines =
        graphPanel.new(
          'Goroutines',
          datasource='$datasource',
          format='short',
        )
        .addTarget(prometheus.target('go_goroutines{%(clusterLabel)s="$cluster",%(kubeletSelector)s,instance=~"$instance"}' % $._config, legendFormat='{{instance}}'));


      dashboard.new(
        '%(dashboardNamePrefix)sKubelet' % $._config.grafanaK8s,
        time_from='now-1h',
        uid=($._config.grafanaDashboardIDs['kubelet.json']),
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
          'label_values(up{%(kubeletSelector)s}, %(clusterLabel)s)' % $._config,
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
          'label_values(up{%(kubeletSelector)s,%(clusterLabel)s="$cluster"}, instance)' % $._config,
          label='instance',
          refresh='time',
          includeAll=true,
          sort=1,
        )
      )
      .addPanel(upCount, gridPos={ h: 7, w: 4, x: 0, y: 0 })
      .addPanel(runningPodCount, gridPos={ h: 7, w: 4, x: 4, y: 0 })
      .addPanel(runningContainerCount, gridPos={ h: 7, w: 4, x: 8, y: 0 })
      .addPanel(actualVolumeCount, gridPos={ h: 7, w: 4, x: 12, y: 0 })
      .addPanel(desiredVolumeCount, gridPos={ h: 7, w: 4, x: 16, y: 0 })
      .addPanel(configErrorCount, gridPos={ h: 7, w: 4, x: 20, y: 0 })
      .addPanel(operationRate, gridPos={ h: 7, w: 12, x: 0, y: 7 })
      .addPanel(operationErrorRate, gridPos={ h: 7, w: 12, x: 12, y: 7 })
      .addPanel(operationLatency, gridPos={ h: 7, w: 24, x: 0, y: 14 })
      .addPanel(podStartRate, gridPos={ h: 7, w: 12, x: 0, y: 21 })
      .addPanel(podStartLatency, gridPos={ h: 7, w: 12, x: 12, y: 21 })
      .addPanel(storageOperationRate, gridPos={ h: 7, w: 12, x: 0, y: 28 })
      .addPanel(storageOperationErrorRate, gridPos={ h: 7, w: 12, x: 12, y: 28 })
      .addPanel(storageOperationLatency, gridPos={ h: 7, w: 24, x: 0, y: 35 })
      .addPanel(cgroupManagerRate, gridPos={ h: 7, w: 12, x: 0, y: 42 })
      .addPanel(cgroupManagerDuration, gridPos={ h: 7, w: 12, x: 12, y: 42 })
      .addPanel(plegRelistRate, gridPos={ h: 7, w: 12, x: 0, y: 49 })
      .addPanel(plegRelistInterval, gridPos={ h: 7, w: 12, x: 12, y: 49 })
      .addPanel(plegRelistDuration, gridPos={ h: 7, w: 24, x: 0, y: 56 })
      .addPanel(rpcRate, gridPos={ h: 7, w: 24, x: 0, y: 63 })
      .addPanel(requestDuration, gridPos={ h: 7, w: 24, x: 0, y: 70 })
      .addPanel(memory, gridPos={ h: 7, w: 8, x: 0, y: 77 })
      .addPanel(cpu, gridPos={ h: 7, w: 8, x: 8, y: 77 })
      .addPanel(goroutines, gridPos={ h: 7, w: 8, x: 16, y: 77 }),
  },
}
