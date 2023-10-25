{
  _config+:: {
    // Tags for dashboards.
    tags: ['pyroscope'],

    // The label used to differentiate between different application instances (i.e. 'pod' in a kubernetes install).
    per_instance_label: 'pod',

    // The label used to differentiate between different nodes (i.e. servers).
    per_node_label: 'instance',

    // The label used to differentiate between different clusters.
    per_cluster_label: 'cluster',
    // Add the mixin with support for multiple cluster
    multi_cluster: true,

    // Regex used to filter proposed datasources
    datasource_regex: '',
  },
}
