local grafana = import 'github.com/grafana/grafonnet-lib/grafonnet/grafana.libsonnet';
local dashboard = grafana.dashboard;
local row = grafana.row;
local prometheus = grafana.prometheus;
local template = grafana.template;
local graphPanel = grafana.graphPanel;

{
  grafanaDashboards+:: {

    local amQuerySelector = std.join(',', ['%s=~"$%s"' % [label, label] for label in std.split($._config.alertmanagerClusterLabels, ',')]),
    local amNameDashboardLegend = std.join('/', ['{{%s}}' % [label] for label in std.split($._config.alertmanagerNameLabels, ',')]),

    local alertmanagerClusterSelectorTemplates =
      [
        template.new(
          name=label,
          label=label,
          datasource='$datasource',
          query='label_values(alertmanager_alerts, %s)' % label,
          current='',
          refresh=2,
          includeAll=false,
          sort=1
        )
        for label in std.split($._config.alertmanagerClusterLabels, ',')
      ],

    local integrationTemplate =
      template.new(
        name='integration',
        datasource='$datasource',
        query='label_values(alertmanager_notifications_total{integration=~"%s"}, integration)' % $._config.alertmanagerCriticalIntegrationsRegEx,
        current='all',
        hide='2',  // Always hide
        refresh=2,
        includeAll=true,
        sort=1
      ),

    'alertmanager-overview.json':
      local alerts =
        graphPanel.new(
          'Alerts',
          description='current set of alerts stored in the Alertmanager',
          datasource='$datasource',
          span=6,
          format='none',
          stack=true,
          fill=1,
          legend_show=false,
        )
        .addTarget(prometheus.target('sum(alertmanager_alerts{%(amQuerySelector)s}) by (%(alertmanagerClusterLabels)s,%(alertmanagerNameLabels)s)' % $._config { amQuerySelector: amQuerySelector }, legendFormat='%(amNameDashboardLegend)s' % $._config { amNameDashboardLegend: amNameDashboardLegend }));

      local alertsRate =
        graphPanel.new(
          'Alerts receive rate',
          description='rate of successful and invalid alerts received by the Alertmanager',
          datasource='$datasource',
          span=6,
          format='ops',
          stack=true,
          fill=1,
          legend_show=false,
        )
        .addTarget(prometheus.target('sum(rate(alertmanager_alerts_received_total{%(amQuerySelector)s}[$__rate_interval])) by (%(alertmanagerClusterLabels)s,%(alertmanagerNameLabels)s)' % $._config { amQuerySelector: amQuerySelector }, legendFormat='%(amNameDashboardLegend)s Received' % $._config { amNameDashboardLegend: amNameDashboardLegend }))
        .addTarget(prometheus.target('sum(rate(alertmanager_alerts_invalid_total{%(amQuerySelector)s}[$__rate_interval])) by (%(alertmanagerClusterLabels)s,%(alertmanagerNameLabels)s)' % $._config { amQuerySelector: amQuerySelector }, legendFormat='%(amNameDashboardLegend)s Invalid' % $._config { amNameDashboardLegend: amNameDashboardLegend }));

      local notifications =
        graphPanel.new(
          '$integration: Notifications Send Rate',
          description='rate of successful and invalid notifications sent by the Alertmanager',
          datasource='$datasource',
          format='ops',
          stack=true,
          fill=1,
          legend_show=false,
          repeat='integration'
        )
        .addTarget(prometheus.target('sum(rate(alertmanager_notifications_total{%(amQuerySelector)s, integration="$integration"}[$__rate_interval])) by (integration,%(alertmanagerClusterLabels)s,%(alertmanagerNameLabels)s)' % $._config { amQuerySelector: amQuerySelector }, legendFormat='%(amNameDashboardLegend)s Total' % $._config { amNameDashboardLegend: amNameDashboardLegend }))
        .addTarget(prometheus.target('sum(rate(alertmanager_notifications_failed_total{%(amQuerySelector)s, integration="$integration"}[$__rate_interval])) by (integration,%(alertmanagerClusterLabels)s,%(alertmanagerNameLabels)s)' % $._config { amQuerySelector: amQuerySelector }, legendFormat='%(amNameDashboardLegend)s Failed' % $._config { amNameDashboardLegend: amNameDashboardLegend }));

      local notificationDuration =
        graphPanel.new(
          '$integration: Notification Duration',
          description='latency of notifications sent by the Alertmanager',
          datasource='$datasource',
          format='s',
          stack=false,
          fill=1,
          legend_show=false,
          repeat='integration'
        )
        .addTarget(prometheus.target(
          |||
            histogram_quantile(0.99,
              sum(rate(alertmanager_notification_latency_seconds_bucket{%(amQuerySelector)s, integration="$integration"}[$__rate_interval])) by (le,%(alertmanagerClusterLabels)s,%(alertmanagerNameLabels)s)
            ) 
          ||| % $._config { amQuerySelector: amQuerySelector }, legendFormat='%(amNameDashboardLegend)s 99th Percentile' % $._config { amNameDashboardLegend: amNameDashboardLegend }
        ))
        .addTarget(prometheus.target(
          |||
            histogram_quantile(0.50,
              sum(rate(alertmanager_notification_latency_seconds_bucket{%(amQuerySelector)s, integration="$integration"}[$__rate_interval])) by (le,%(alertmanagerClusterLabels)s,%(alertmanagerNameLabels)s)
            ) 
          ||| % $._config { amQuerySelector: amQuerySelector }, legendFormat='%(amNameDashboardLegend)s Median' % $._config { amNameDashboardLegend: amNameDashboardLegend }
        ))
        .addTarget(prometheus.target(
          |||
            sum(rate(alertmanager_notification_latency_seconds_sum{%(amQuerySelector)s, integration="$integration"}[$__rate_interval])) by (%(alertmanagerClusterLabels)s,%(alertmanagerNameLabels)s)
            /
            sum(rate(alertmanager_notification_latency_seconds_count{%(amQuerySelector)s, integration="$integration"}[$__rate_interval])) by (%(alertmanagerClusterLabels)s,%(alertmanagerNameLabels)s)
          ||| % $._config { amQuerySelector: amQuerySelector }, legendFormat='%(amNameDashboardLegend)s Average' % $._config { amNameDashboardLegend: amNameDashboardLegend }
        ));

      dashboard.new(
        '%sOverview' % $._config.dashboardNamePrefix,
        time_from='now-1h',
        tags=($._config.dashboardTags),
        timezone='utc',
        refresh='30s',
        graphTooltip='shared_crosshair',
        uid='alertmanager-overview'
      )
      .addTemplate(
        {
          current: {
            text: 'Prometheus',
            value: 'Prometheus',
          },
          hide: 0,
          label: 'Data Source',
          name: 'datasource',
          options: [],
          query: 'prometheus',
          refresh: 1,
          regex: '',
          type: 'datasource',
        },
      )
      .addTemplates(alertmanagerClusterSelectorTemplates)
      .addTemplate(integrationTemplate)
      .addRow(
        row.new('Alerts')
        .addPanel(alerts)
        .addPanel(alertsRate)
      )
      .addRow(
        row.new('Notifications')
        .addPanel(notifications)
        .addPanel(notificationDuration)
      ),
  },
}
