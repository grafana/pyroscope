local prometheus = import 'prometheus/prometheus.libsonnet';
local scrape_configs = import 'prometheus/scrape_configs.libsonnet';

{
  prometheus_config:: {
    global: {
      scrape_interval: '15s',
    },

    rule_files: [
      'alerts/alerts.rules',
      'recording/recording.rules',
    ],

    alerting: {
      alertmanagers: prometheus.withAlertmanagers(
        $._config.alertmanagers,
        $._config.cluster_name
      ).prometheus_config.alerting.alertmanagers,
    },

    scrape_configs: [
      // Grafana Labs' battle tested scrape config for scraping kubernetes pods.
      scrape_configs.kubernetes_pods,

      // kube-dns does not adhere to the conventions set out by
      // `scrape_configs.kubernetes_pods`.
      scrape_configs.kube_dns,

      // This scrape config gathers all kubelet metrics.
      scrape_configs.kubelet($._config.prometheus_api_server_address)
      + (
        // Couldn't get prometheus to validate the kubelet cert for scraping, so
        // don't bother for now.
        if $._config.prometheus_insecure_skip_verify
        then scrape_configs.insecureSkipVerify
        else {}
      ),

      // This scrape config gathers cAdvisor metrics.
      scrape_configs.cadvisor($._config.prometheus_api_server_address)
      + (
        if $._config.prometheus_insecure_skip_verify
        then scrape_configs.insecureSkipVerify
        else {}
      ),

      // If running on GKE, you cannot scrape API server pods, and must instead
      // scrape the API server service endpoints.  On AKS this doesn't work.
      (
        if $._config.scrape_api_server_endpoints
        then scrape_configs.kubernetes_api(role='endpoints')  // GKE
        else scrape_configs.kubernetes_api(role='service')  // AKS et al.
      )
      + (
        if $._config.prometheus_insecure_skip_verify
        then scrape_configs.insecureSkipVerify
        else {}
      ),
    ],
  },
}
