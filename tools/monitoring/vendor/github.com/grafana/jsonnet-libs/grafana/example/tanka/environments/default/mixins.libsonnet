local node_exporter = import 'node-mixin/mixin.libsonnet';

{
  node_exporter:
    node_exporter {
      _config+:: {
        nodeExporterSelector: 'job="kubernetes-service-endpoints"',
      },
      grafanaDashboardFolder: 'Node Exporter Mixin',
    },
}
