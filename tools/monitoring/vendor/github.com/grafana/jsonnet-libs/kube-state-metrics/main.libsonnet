local kausal = import 'github.com/grafana/jsonnet-libs/ksonnet-util/kausal.libsonnet';

{
  new(
    namespace,
    image='k8s.gcr.io/kube-state-metrics/kube-state-metrics:v2.1.0',
  ):: {
    local k = kausal {
      _config+: {
        namespace: namespace,
      },
    },

    local container = k.core.v1.container,
    local containerPort = k.core.v1.containerPort,
    container::
      container.new('kube-state-metrics', image)
      + container.withArgs([
        '--port=8080',
        '--telemetry-host=0.0.0.0',
        '--telemetry-port=8081',
      ])
      + container.withPorts([
        containerPort.new('ksm', 8080),
        containerPort.new('self-metrics', 8081),
      ])
      + k.util.resourcesRequests('50m', '50Mi')
      + k.util.resourcesLimits('250m', '150Mi'),

    local deployment = k.apps.v1.deployment,
    deployment:
      deployment.new('kube-state-metrics', 1, [self.container])
      + deployment.mixin.spec.template.spec.withServiceAccountName(self.rbac.service_account.metadata.name)
      + deployment.mixin.spec.template.spec.securityContext.withRunAsUser(65534)
      + deployment.mixin.spec.template.spec.securityContext.withRunAsGroup(65534),

    local policyRule = k.rbac.v1.policyRule,
    rbac:
      k.util.rbac('kube-state-metrics', [
        policyRule.new()
        + policyRule.withApiGroups([''])
        + policyRule.withResources([
          'configmaps',
          'secrets',
          'nodes',
          'pods',
          'services',
          'resourcequotas',
          'replicationcontrollers',
          'limitranges',
          'persistentvolumeclaims',
          'persistentvolumes',
          'namespaces',
          'endpoints',
        ])
        + policyRule.withVerbs(['list', 'watch']),

        policyRule.new()
        + policyRule.withApiGroups(['extensions'])
        + policyRule.withResources([
          'daemonsets',
          'deployments',
          'replicasets',
          'ingresses',
        ])
        + policyRule.withVerbs(['list', 'watch']),

        policyRule.new()
        + policyRule.withApiGroups(['apps'])
        + policyRule.withResources([
          'daemonsets',
          'deployments',
          'replicasets',
          'statefulsets',
        ])
        + policyRule.withVerbs(['list', 'watch']),

        policyRule.new()
        + policyRule.withApiGroups(['batch'])
        + policyRule.withResources([
          'cronjobs',
          'jobs',
        ])
        + policyRule.withVerbs(['list', 'watch']),

        policyRule.new()
        + policyRule.withApiGroups(['autoscaling'])
        + policyRule.withResources([
          'horizontalpodautoscalers',
        ])
        + policyRule.withVerbs(['list', 'watch']),

        policyRule.new()
        + policyRule.withApiGroups(['authorization.k8s.io'])
        + policyRule.withResources(['subjectaccessreviews'])
        + policyRule.withVerbs(['create']),

        policyRule.new()
        + policyRule.withApiGroups(['ingresses'])
        + policyRule.withResources(['ingress'])
        + policyRule.withVerbs(['list', 'watch']),

        policyRule.new()
        + policyRule.withApiGroups(['policy'])
        + policyRule.withResources(['poddisruptionbudgets'])
        + policyRule.withVerbs(['list', 'watch']),

        policyRule.new()
        + policyRule.withApiGroups(['certificates.k8s.io'])
        + policyRule.withResources(['certificatesigningrequests'])
        + policyRule.withVerbs(['list', 'watch']),

        policyRule.new()
        + policyRule.withApiGroups(['storage.k8s.io'])
        + policyRule.withResources([
          'storageclasses',
          'volumeattachments',
        ])
        + policyRule.withVerbs(['list', 'watch']),

        policyRule.new()
        + policyRule.withApiGroups(['admissionregistration.k8s.io'])
        + policyRule.withResources([
          'mutatingwebhookconfigurations',
          'validatingwebhookconfigurations',
        ])
        + policyRule.withVerbs(['list', 'watch']),

        policyRule.new()
        + policyRule.withApiGroups(['networking.k8s.io'])
        + policyRule.withResources([
          'networkpolicies',
          'ingresses',
        ])
        + policyRule.withVerbs(['list', 'watch']),

        policyRule.new()
        + policyRule.withApiGroups(['coordination.k8s.io'])
        + policyRule.withResources(['leases'])
        + policyRule.withVerbs(['list', 'watch']),
      ]),
  },

  scrape_config: (import './scrape_config.libsonnet'),
}
