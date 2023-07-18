local k = import 'ksonnet-util/kausal.libsonnet';
local deployment = k.apps.v1.deployment;
local statefulset = k.apps.v1.statefulSet;
local service = k.core.v1.service;
local servicePort = service.mixin.spec.portsType;
local container = k.core.v1.container;
{
  grafana_container::
    container.new('grafana', $._images.grafana) +
    container.withPorts(k.core.v1.containerPort.new('grafana-metrics', $._config.containerPort)) +
    container.withEnvMap({
      GF_PATHS_CONFIG: '/etc/grafana-config/grafana.ini',
      GF_INSTALL_PLUGINS: std.join(',', $.grafanaPlugins),
    }) +
    k.util.resourcesRequests('10m', '40Mi'),

  grafana_deployment:
    deployment.new('grafana', $._config.replicas, [$.grafana_container])
    + $.configmap_mounts
    + k.util.podPriority('critical'),

  grafana_service:
    k.util.serviceFor($.grafana_deployment) +
    service.mixin.spec.withPortsMixin([
      servicePort.newNamed(
        name='http',
        port=$._config.port,
        targetPort=$._config.containerPort,
      ),
    ]),
}
