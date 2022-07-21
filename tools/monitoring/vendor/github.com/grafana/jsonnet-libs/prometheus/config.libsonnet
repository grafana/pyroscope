{
  _config+:: {
    name:: 'prometheus',

    // Cluster and environment specific overrides.
    cluster_dns_tld: 'local.',
    cluster_dns_suffix: 'cluster.' + self.cluster_dns_tld,
    namespace: error 'must specify namespace',

    // Prometheus config basics
    prometheus_requests_cpu: '250m',
    prometheus_requests_memory: '1536Mi',
    prometheus_limits_cpu: '500m',
    prometheus_limits_memory: '2Gi',

    // Prometheus config options.
    prometheus_external_hostname: 'http://prometheus.%(namespace)s.svc.%(cluster_dns_suffix)s' % self,
    prometheus_path: '/prometheus/',
    prometheus_port: 9090,
    prometheus_web_route_prefix: self.prometheus_path,
    prometheus_config_dir: '/etc/prometheus',
    prometheus_config_file: self.prometheus_config_dir + '/prometheus.yml',
    prometheus_enabled_features: ['exemplar-storage'],
  },

  scrape_configs: {},

  addScrapeConfig(scrape_config):: {
    scrape_configs+:: {
      [scrape_config.job_name]: scrape_config,
    },
  },

  prometheus_config:: {
    global: {
      scrape_interval: '15s',
    },

    rule_files: [
      'alerts/alerts.rules',
      'recording/recording.rules',
    ],

    scrape_configs+: [
      $.scrape_configs[k]
      for k in std.objectFields($.scrape_configs)
    ],
  },

  // `withAlertmanagers` adds an alertmanager configuration to
  // prometheus. It is intended to work with `buildPeers` in the
  // alertmanager jsonnet lib to provide one global alertmanager
  // über-cluster spread over multiple kubernetes clusters. This
  // requires all those clusters to have inter-cluster network
  // connectivity.
  //
  // ref: https://github.com/grafana/jsonnet-libs/tree/master/alertmanager
  //
  // `global` is set to 'true' if the alertmanager is participating in
  // the global alertmanager über-cluster.
  //
  // `cluster_name` has 2 functions:
  // 1. Prometheus will prefer looking up the alertmanagers through
  //    k8s service discovery if the alertmanager is running in the
  //    same cluster, this will be done by comparing `cluster_name`.
  // 2. If the `cluster_name` does not match, then this will be used
  //    to construct the URLs to the alertmanager pods to fill the
  //    static_configs.
  //
  // `withAlertmanagers` is intended to be compatible with `buildPeers`
  // in the alertmanager jsonnet lib.
  //
  // Example `alertmanagers` object:
  // alertmanagers: {
  //   alertmanager_name: {
  //     // path prefix where the alertmanager is running
  //     path: '/alertmanager/',
  //
  //     // for service discovery and global static config
  //     namespace: 'alertmanager',
  //     port: 9093,
  //
  //     // for global static config
  //     replicas: 2,
  //     global: true,
  //     cluster_name: 'us-central1',
  //     cluster_dns_tld: 'local',
  //   },
  // }
  withAlertmanagers(alertmanagers, cluster_name):: {
    prometheus_config+: {
      alerting+: {
        alertmanagers: std.prune(
          [
            local alertmanager = alertmanagers[am];

            // For local alertmanager or local instances of the global alertmanager, use K8s SD.
            if cluster_name == alertmanager.cluster_name &&
               alertmanager.replicas > 0
            then {
              api_version: 'v2',
              kubernetes_sd_configs: [{
                role: 'pod',
              }],
              path_prefix: alertmanager.path,

              relabel_configs: [{
                source_labels: ['__meta_kubernetes_pod_label_name'],
                regex: 'alertmanager',
                action: 'keep',
              }, {
                source_labels: ['__meta_kubernetes_namespace'],
                regex: alertmanager.namespace,
                action: 'keep',
              }, {
                // This prevents port-less containers and the gossip ports
                // from showing up.
                source_labels: ['__meta_kubernetes_pod_container_port_number'],
                regex: alertmanager.port,
                action: 'keep',
              }],
            }
            else
              // For non-local instances, use static DNS entries.
              if alertmanager.global &&
                 alertmanager.replicas > 0
              then {
                api_version: 'v2',
                path_prefix: alertmanager.path,
                static_configs: [{
                  targets: [
                    'alertmanager-%d.alertmanager.%s.svc.%s.%s:%s' % [
                      i,
                      alertmanager.namespace,
                      alertmanager.cluster_name,
                      alertmanager.cluster_dns_tld,
                      alertmanager.port,
                    ]
                    for i in std.range(0, alertmanager.replicas - 1)
                  ],
                }],
              }
              else {}
            for am in std.objectFields(alertmanagers)
          ]
        ),
      },
    },
  },
}
