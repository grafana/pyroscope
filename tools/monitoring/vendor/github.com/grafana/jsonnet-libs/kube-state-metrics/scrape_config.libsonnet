// A separate scrape config for kube-state-metrics which doesn't
// add namespace, container, and pod labels, instead taking
// those labels from the exported timeseries. This prevents them
// being renamed to exported_namespace etc.  and allows us to
// route alerts based on namespace and join KSM metrics with
// cAdvisor metrics.
function(namespace) {
  job_name: '%s/kube-state-metrics' % namespace,
  kubernetes_sd_configs: [{
    role: 'pod',
    namespaces: {
      names: [namespace],
    },
  }],

  relabel_configs: [

    // Drop anything whose service is not kube-state-metrics.
    {
      source_labels: ['__meta_kubernetes_pod_label_name'],
      regex: 'kube-state-metrics',
      action: 'keep',
    },

    // Drop anything whose port is not 'ksm', these are the metrics computed by
    // kube-state-metrics itself and not the 'self metrics' which should be
    // scraped by normal prometheus service discovery ('self-metrics' port
    // name).
    {
      source_labels: ['__meta_kubernetes_pod_container_port_name'],
      regex: 'ksm',
      action: 'keep',
    },

    // Rename instances to the concatenation of pod:container:port.
    // In the specific case of KSM, we could leave out the container
    // name and still have a unique instance label, but we leave it
    // in here for consistency with the normal pod scraping.
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
}
