{
  _config+:: {
    // Cluster and environment specific overrides.
    cluster_dns_tld: 'local.',
    cluster_dns_suffix: 'cluster.' + self.cluster_dns_tld,
    cluster_name: error 'must specify cluster name',
    namespace: error 'must specify namespace',
    alertmanager_namespace: self.namespace,
    grafana_namespace: self.namespace,
    prometheus_namespace: self.namespace,
    node_exporter_namespace: self.namespace,
    kube_state_metrics_namespace: self.namespace,

    // Grafana config options.
    grafana_root_url: 'http://nginx.%(namespace)s.svc.%(cluster_dns_suffix)s/grafana' % self,

    // Overrides for the nginx frontend for all these services.
    admin_services+: std.prune([
      {
        title: 'Prometheus',
        path: 'prometheus',
        url: 'http://prometheus.%(prometheus_namespace)s.svc.%(cluster_dns_suffix)s/prometheus/' % $._config,
      },
      if $._config.alertmanager_cluster_self.replicas > 0 then {
        title: 'Alertmanager' + if $._config.alertmanager_cluster_self.global then ' (global)' else ' (local)',
        path: 'alertmanager',
        url: 'http://alertmanager.%(alertmanager_namespace)s.svc.%(cluster_dns_suffix)s/alertmanager/' % $._config,
      },
    ]),

    // Prometheus config options - DEPRECATED, for backwards compatibility.
    apiServerAddress: 'kubernetes.default.svc.%(cluster_dns_suffix)s:443' % self,
    insecureSkipVerify: false,

    // Prometheus config basics
    prometheus_requests_cpu: '250m',
    prometheus_requests_memory: '1536Mi',
    prometheus_limits_cpu: '500m',
    prometheus_limits_memory: '2Gi',

    // Prometheus config options.
    prometheus_api_server_address: self.apiServerAddress,
    scrape_api_server_endpoints: true,
    prometheus_insecure_skip_verify: self.insecureSkipVerify,
    prometheus_external_hostname: 'http://prometheus.%(namespace)s.svc.%(cluster_dns_suffix)s' % self,
    prometheus_path: '/prometheus/',
    prometheus_port: 9090,
    prometheus_web_route_prefix: self.prometheus_path,
    prometheus_config_dir: '/etc/prometheus',
    prometheus_config_file: self.prometheus_config_dir + '/prometheus.yml',

    // Alertmanager config options.
    alertmanager_external_hostname: 'http://alertmanager.%(namespace)s.svc.%(cluster_dns_suffix)s' % self,
    alertmanager_path: '/alertmanager/',
    alertmanager_port: 9093,
    alertmanager_gossip_port: 9094,
    // Description of how many alertmanager replicas to run where. All
    // clusters with `'global': true` are participating in one global
    // alertmanager über-cluster, which requires all those clusters to
    // have inter-cluster network connectivity. Configure 2–3 clusters
    // with 2–3 replicas each to limit global gossiping. Clusters that
    // should not or cannot participate in the alertmanager
    // über-cluster must be set to `'global': false`. That's usually
    // the case for clusters without inter-cluster network
    // connectivity. The alertmanager instances in those clusters only
    // gossip with others in the same clusters (or not at all if
    // `'replicas': 0`). Prometheus servers in those cluster only talk
    // to the local alertmanager instances. In all other clusters, the
    // Prometheus servers talk to all clustered alertmanager replicas
    // globally.
    alertmanager_clusters: {
      // Example for a cluster with global alertmanager instances:
      //   'us-east5': { global: true, replicas: 2 },
      // Example for cluster with isolated local alertmanager instance:
      //   'eu-west7': { global: false, replicas: 1 },
      // This is the default for all clusters not mentioned, i.e. let
      // Prometheus servers talk to global alertmanagers, don't have
      // own alertmanagers in this cluster:
      //   'us-west3': { global: true, replicas: 0 },
    },

    // This replaces `alertmanager_clusters` to be more granular, this
    // `alertmanagers` object is build from `alertmanager_clusters`
    // for backwards compatibility. It is intended to be consumed by
    // functions in the prometheus (withAlertmanagers) and
    // alertmanager (buildPeers) jsonnet libs.
    alertmanagers: {
      [c]: {
        // path prefix where the alertmanager is running
        path: $._config.alertmanager_path,

        // for service discovery and global static config
        namespace: $._config.alertmanager_namespace,
        port: $._config.alertmanager_port,
        gossip_port: $._config.alertmanager_gossip_port,

        // for global static config
        replicas: $._config.alertmanager_clusters[c].replicas,
        global: $._config.alertmanager_clusters[c].global,
        cluster_name: c,
        cluster_dns_tld: $._config.cluster_dns_tld,
      }
      for c in std.objectFields($._config.alertmanager_clusters)
    },

    // Backwards compatible base entry for $.alertmanager_cluster_self
    local alertmanager_config_base = {
      path: $._config.alertmanager_path,
      namespace: $._config.alertmanager_namespace,
      port: $._config.alertmanager_port,
      gossip_port: $._config.alertmanager_gossip_port,
      cluster_name: $._config.cluster_name,
      cluster_dns_tld: $._config.cluster_dns_tld,
    },

    // Shortcut to alertmanagers entry for this cluster.
    alertmanager_cluster_self:
      if self.cluster_name in self.alertmanagers then
        self.alertmanagers[self.cluster_name]
      else if std.length(self.alertmanagers) == 0 then
        alertmanager_config_base { global: false, replicas: 1 }
      else
        alertmanager_config_base { global: true, replicas: 0 },
    slack_url: 'http://slack',
    slack_channel: 'general',

    // Node exporter options.
    node_exporter_mount_root: true,

    // oauth2-proxy
    oauth_enabled: false,

    // Nginx proxy_read_timeout (in seconds) 60s is the nginx default
    nginx_proxy_read_timeout: '60',
    // Nginx proxy_send_timeout (in seconds) 60s is the nginx default
    nginx_proxy_send_timeout: '60',
  },
}
