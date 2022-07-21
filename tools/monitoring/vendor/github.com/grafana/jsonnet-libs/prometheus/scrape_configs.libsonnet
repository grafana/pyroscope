// Grafana Labs' battle tested scrape configs for scraping kubernetes.

{
  // Generic scrape config for scraping kubernetes pods.
  //
  // Scraping happens on following characteristics:
  //   - on named Pod ports that end on '-metrics'
  //   - Pod has a 'name' label
  //   - Pod status is not Succeeded/Failed
  //
  // You can specify the following annotations (on pods):
  //   prometheus.io.scrape: false - don't scrape this pod
  //   prometheus.io.scheme: https - use https for scraping
  //   prometheus.io.path - scrape this path
  //   prometheus.io.port - scrape this port
  //   prometheus.io.param-<parameter> - send ?parameter=value with the scrape
  kubernetes_pods:: {
    job_name: 'kubernetes-pods',
    kubernetes_sd_configs: [{
      role: 'pod',
    }],

    relabel_configs: [

      // Drop any endpoint whose pod port name does not end with metrics.
      {
        source_labels: ['__meta_kubernetes_pod_container_port_name'],
        action: 'keep',
        regex: '.*-metrics',
      },

      // Drop pods without a name label.
      {
        source_labels: ['__meta_kubernetes_pod_label_name'],
        action: 'drop',
        regex: '',
      },

      // Drop pods with phase Succeeded or Failed.
      {
        source_labels: ['__meta_kubernetes_pod_phase'],
        action: 'drop',
        regex: 'Succeeded|Failed',
      },

      // Drop anything annotated with 'prometheus.io.scrape=false'.
      {
        source_labels: ['__meta_kubernetes_pod_annotation_prometheus_io_scrape'],
        action: 'drop',
        regex: 'false',
      },

      // Allow pods to override the scrape scheme with 'prometheus.io.scheme=https'.
      {
        source_labels: ['__meta_kubernetes_pod_annotation_prometheus_io_scheme'],
        action: 'replace',
        target_label: '__scheme__',
        regex: '(https?)',
        replacement: '$1',
      },

      // Allow service to override the scrape path with 'prometheus.io.path=/other_metrics_path'.
      {
        source_labels: ['__meta_kubernetes_pod_annotation_prometheus_io_path'],
        action: 'replace',
        target_label: '__metrics_path__',
        regex: '(.+)',
        replacement: '$1',
      },

      // Allow services to override the scrape port with 'prometheus.io.port=1234'.
      {
        source_labels: ['__address__', '__meta_kubernetes_pod_annotation_prometheus_io_port'],
        action: 'replace',
        target_label: '__address__',
        regex: '(.+?)(\\:\\d+)?;(\\d+)',
        replacement: '$1:$3',
      },

      // Map all K8s labels/annotations starting with
      // 'prometheus.io/param-' to URL params for Prometheus scraping.
      {
        regex: '__meta_kubernetes_pod_annotation_prometheus_io_param_(.+)',
        action: 'labelmap',
        replacement: '__param_$1',
      },

      // Map all K8s labels/annotations starting with
      // 'prometheus.io/label-' to Prometheus labels.
      {
        regex: '__meta_kubernetes_pod_label_prometheus_io_label_(.+)',
        action: 'labelmap',
      },
      {
        regex: '__meta_kubernetes_pod_annotation_prometheus_io_label_(.+)',
        action: 'labelmap',
      },

      // Rename jobs to be <namespace>/<name, from pod name label>.
      {
        source_labels: ['__meta_kubernetes_namespace', '__meta_kubernetes_pod_label_name'],
        action: 'replace',
        separator: '/',
        target_label: 'job',
        replacement: '$1',
      },

      // But also include the namespace, container, pod as separate labels,
      // for routing alerts and joining with cAdvisor metrics.
      {
        source_labels: ['__meta_kubernetes_namespace'],
        action: 'replace',
        target_label: 'namespace',
      },
      {
        source_labels: ['__meta_kubernetes_pod_name'],
        action: 'replace',
        target_label: 'pod',  // Not 'pod_name', which disappeared in K8s 1.16.
      },
      {
        source_labels: ['__meta_kubernetes_pod_container_name'],
        action: 'replace',
        target_label: 'container',  // Not 'container_name', which disappeared in K8s 1.16.
      },

      // Rename instances to the concatenation of 'pod:container:port',
      // all three components are needed to guarantee a unique instance label.
      {
        source_labels: [
          '__meta_kubernetes_pod_name',
          '__meta_kubernetes_pod_container_name',
          '__meta_kubernetes_pod_container_port_name',
        ],
        action: 'replace',
        separator: ':',
        target_label: 'instance',
      },

    ],
  },

  // Gather kube-dns metrics.
  kube_dns:: {
    job_name: 'kube-system/kube-dns',
    kubernetes_sd_configs: [{
      role: 'pod',
      namespaces: {
        names: ['kube-system'],
      },
    }],

    relabel_configs: [

      // Scrape only kube-dns.
      {
        source_labels: ['__meta_kubernetes_pod_label_k8s_app'],
        action: 'keep',
        regex: 'kube-dns',
      },

      // Scrape the ports named "metrics".
      {
        source_labels: ['__meta_kubernetes_pod_container_port_name'],
        action: 'keep',
        regex: 'metrics',
      },

      // Include the namespace, container, pod as separate labels,
      // for routing alerts and joining with cAdvisor metrics.
      {
        source_labels: ['__meta_kubernetes_namespace'],
        action: 'replace',
        target_label: 'namespace',
      },
      {
        source_labels: ['__meta_kubernetes_pod_name'],
        action: 'replace',
        target_label: 'pod',  // Not 'pod_name', which disappeared in K8s 1.16.
      },
      {
        source_labels: ['__meta_kubernetes_pod_container_name'],
        action: 'replace',
        target_label: 'container',  // Not 'container_name', which disappeared in K8s 1.16.
      },

      // Rename instances to the concatenation of 'pod:container:port',
      // all three components are needed to guarantee a unique instance label.
      {
        source_labels: [
          '__meta_kubernetes_pod_name',
          '__meta_kubernetes_pod_container_name',
          '__meta_kubernetes_pod_container_port_name',
        ],
        action: 'replace',
        separator: ':',
        target_label: 'instance',
      },
    ],
  },

  // Gather all kubelet metrics.
  kubelet(api_server_address):: {
    job_name: 'kube-system/kubelet',
    bearer_token_file: '/var/run/secrets/kubernetes.io/serviceaccount/token',
    tls_config: {
      ca_file: '/var/run/secrets/kubernetes.io/serviceaccount/ca.crt',
    },
    kubernetes_sd_configs: [{
      role: 'node',
    }],

    relabel_configs: [
      {
        target_label: '__address__',
        replacement: api_server_address,
      },
      {
        target_label: '__scheme__',
        replacement: 'https',
      },
      {
        source_labels: ['__meta_kubernetes_node_name'],
        regex: '(.+)',
        target_label: '__metrics_path__',
        replacement: '/api/v1/nodes/${1}/proxy/metrics',
      },
    ],
  },

  // Gather cAdvisor metrics.
  cadvisor(api_server_address):: {
    job_name: 'kube-system/cadvisor',
    bearer_token_file: '/var/run/secrets/kubernetes.io/serviceaccount/token',
    tls_config: {
      ca_file: '/var/run/secrets/kubernetes.io/serviceaccount/ca.crt',
    },
    kubernetes_sd_configs: [{
      role: 'node',
    }],
    scheme: 'https',

    relabel_configs: [
      {
        target_label: '__address__',
        replacement: api_server_address,
      },

      // cAdvisor metrics are available via kubelet using the /metrics/cadvisor path.
      {
        source_labels: ['__meta_kubernetes_node_name'],
        regex: '(.+)',
        target_label: '__metrics_path__',
        replacement: '/api/v1/nodes/${1}/proxy/metrics/cadvisor',
      },
    ],

    metric_relabel_configs: [
      // Drop container_* metrics with no image.
      {
        source_labels: ['__name__', 'image'],
        regex: 'container_([a-z_]+);',
        action: 'drop',
      },

      // Drop a bunch of metrics which are disabled but still sent,
      // see https://github.com/google/cadvisor/issues/1925.
      {
        source_labels: ['__name__'],
        regex: 'container_(network_tcp_usage_total|network_udp_usage_total|tasks_state|cpu_load_average_10s)',
        action: 'drop',
      },
    ],
  },

  // Gather kubernetes API metrics.
  kubernetes_api(role='endpoints'):: {
    job_name: 'default/kubernetes',
    bearer_token_file: '/var/run/secrets/kubernetes.io/serviceaccount/token',
    tls_config: {
      ca_file: '/var/run/secrets/kubernetes.io/serviceaccount/ca.crt',
    },
    kubernetes_sd_configs: [{
      // On GKE you cannot scrape API server pods, and must instead scrape the
      // API server service endpoints.  On AKS this doesn't work and you must
      // use role='service'.
      role: role,
    }],
    scheme: 'https',

    relabel_configs: [{
      source_labels: ['__meta_kubernetes_service_label_component'],
      regex: 'apiserver',
      action: 'keep',
    }],

    // Drop some high cardinality metrics.
    metric_relabel_configs: [
      {
        source_labels: ['__name__'],
        regex: 'apiserver_admission_controller_admission_latencies_seconds_.*',
        action: 'drop',
      },
      {
        source_labels: ['__name__'],
        regex: 'apiserver_admission_step_admission_latencies_seconds_.*',
        action: 'drop',
      },
    ],
  },

  // Don't verify the SSL certificate.
  // Intended to be mixed in with a scrape config.
  insecureSkipVerify:: {
    tls_config+: {
      insecure_skip_verify: true,
    },
  },
}
