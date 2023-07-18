local k = import 'ksonnet-util/kausal.libsonnet';
local kube_state_metrics = import 'kube-state-metrics/main.libsonnet';

{
  local ksm = kube_state_metrics.new(
    $._config.namespace,
    $._images.kubeStateMetrics,
  ),

  kube_state_metrics_rbac: ksm.rbac,

  kube_state_metrics_container:: ksm.container,

  kube_state_metrics_deployment:
    ksm.deployment
    + k.apps.v1.deployment.spec.template.spec.withContainers([$.kube_state_metrics_container])
    + k.util.podPriority('critical'),

  kube_state_metrics_service:
    k.util.serviceFor($.kube_state_metrics_deployment),

  prometheus_config+:: {
    scrape_configs+: [
      kube_state_metrics.scrape_config($._config.kube_state_metrics_namespace),
    ],
  },
}
