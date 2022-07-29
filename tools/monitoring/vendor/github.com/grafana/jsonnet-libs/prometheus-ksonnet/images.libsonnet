local alertmanager_images = import 'alertmanager/images.libsonnet';
local prometheus_images = import 'prometheus/images.libsonnet';

{
  _images+::
    prometheus_images +
    alertmanager_images +
    {
      watch: 'weaveworks/watch:master-5fc29a9',
      kubeStateMetrics: 'k8s.gcr.io/kube-state-metrics/kube-state-metrics:v2.1.0',
      nodeExporter: 'prom/node-exporter:v1.3.1',
      nginx: 'nginx:1.15.1-alpine',
    },
}
