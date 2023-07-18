{
  _config+:: {
    // configure Grafana Lib based upon legacy configs below
    rootUrl: $._config.grafana_root_url,
    provisioningDir: $._config.grafana_provisioning_dir,

    labels+: {
      dashboards: $._config.grafana_dashboard_labels,
      datasources: $._config.grafana_dashboard_labels,
      notificationChannels: $._config.grafana_notification_channel_labels,
    },

    grafana_ini+: {
      sections+: {
        'auth.anonymous': {
          enabled: true,
          org_role: 'Admin',
        },
      },

    } + $.grafana_config,  //<--legacy config

    // LEGACY CONFIGS:
    // Grafana config options.
    grafana_root_url: '',
    grafana_provisioning_dir: '/etc/grafana/provisioning',

    // Optionally add labels to grafana config maps.
    grafana_dashboard_labels: {},
    grafana_datasource_labels: {},
    grafana_notification_channel_labels: {},
  },

  // legacy grafana_ini extension point
  grafana_config+:: {},
}
