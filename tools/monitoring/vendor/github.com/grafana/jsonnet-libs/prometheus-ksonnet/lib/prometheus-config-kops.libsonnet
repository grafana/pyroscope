{
  local k8s_pod_scrape(name, port) = {
    job_name: name,
    kubernetes_sd_configs: [{
      role: 'pod',
    }],

    tls_config: {
      ca_file: '/var/run/secrets/kubernetes.io/serviceaccount/ca.crt',
      insecure_skip_verify: $._config.insecureSkipVerify,
    },
    bearer_token_file: '/var/run/secrets/kubernetes.io/serviceaccount/token',

    relabel_configs: [
      // Only keeps jobs matching <namespace>/<k8s_app, from pod name label>
      {
        source_labels: ['__meta_kubernetes_namespace', '__meta_kubernetes_pod_label_k8s_app'],
        separator: '/',
        action: 'keep',
        regex: name,
      },

      // Rename instances to be the pod name
      {
        source_labels: ['__meta_kubernetes_pod_name'],
        action: 'replace',
        target_label: 'instance',
      },

      // Override the port.
      {
        source_labels: ['__address__'],
        action: 'replace',
        target_label: '__address__',
        regex: '(.+?)(\\:\\d+)?',
        replacement: '$1:%s' % port,
      },

      // But also include the namespace as a separate label, for routing alerts
      {
        source_labels: ['__meta_kubernetes_namespace'],
        action: 'replace',
        target_label: 'namespace',
      },
    ],
  },

  prometheus_config+:: {
    scrape_configs+: [
      k8s_pod_scrape('kube-system/kube-apiserver', 443) {
        scheme: 'https',
      },

      k8s_pod_scrape('kube-system/kube-scheduler', 10251),
      k8s_pod_scrape('kube-system/kube-controller-manager', 10252),

      // kops doesn't configure kube-proxy to listen on non-localhost,
      // can't scrape.
      // k8s_pod_scrape("kube-system/kube-proxy", 10249),

      // kops firewalls etcd on masters off from nodes, so we
      // can't scrape it with Prometheus.
      // k8s_pod_scrape("kube-system/etcd-server", 4001),
      // k8s_pod_scrape("kube-system/etcd-server-events", 4002),
    ],
  },
}
