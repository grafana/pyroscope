// A scrape config for node-exporter which maps the nodename onto the
// instance label.
function(namespace) {
  job_name: '%s/node-exporter' % namespace,
  kubernetes_sd_configs: [{
    role: 'pod',
    namespaces: {
      names: [namespace],
    },
  }],

  relabel_configs: [
    // Drop anything whose name is not node-exporter.
    {
      source_labels: ['__meta_kubernetes_pod_label_name'],
      regex: 'node-exporter',
      action: 'keep',
    },

    // Rename instances to be the node name.
    {
      source_labels: ['__meta_kubernetes_pod_node_name'],
      action: 'replace',
      target_label: 'instance',
    },

    // But also include the namespace as a separate label, for routing alerts.
    {
      source_labels: ['__meta_kubernetes_namespace'],
      action: 'replace',
      target_label: 'namespace',
    },
  ],
}
