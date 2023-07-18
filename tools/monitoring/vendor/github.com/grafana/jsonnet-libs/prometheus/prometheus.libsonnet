local ha_mixin = import 'ha-mixin.libsonnet';
local kausal = import 'ksonnet-util/kausal.libsonnet';

(import 'config.libsonnet')
+ (import 'images.libsonnet')
+ (import 'mixins.libsonnet')
+ {
  local this = self,
  local _config = self._config,
  local k = kausal {
    _config+:: _config,
  } + (
    // an attempt at providing compat with the original ksonnet-lib
    if std.objectHas(kausal, '__ksonnet')
    then
      {
        core+: { v1+: {
          servicePort: kausal.core.v1.service.mixin.spec.portsType,
        } },
        rbac+: { v1+: {
          policyRule: kausal.rbac.v1.clusterRole.rulesType,
        } },
      }
    else {}
  ),

  prometheusAlerts+:: {},
  prometheusRules+:: {},

  withHighAvailability(replicas=2):: ha_mixin(replicas),

  local configMap = k.core.v1.configMap,

  prometheus_config_maps:
    // Can't reference self.foo below as we're in a map context, so
    // need to capture reference to the configs in scope here.
    local prometheus_config = self.prometheus_config;
    local prometheusAlerts = self.prometheusAlerts;
    local prometheusRules = self.prometheusRules;
    [
      configMap.new('%s-config' % _config.name)
      + configMap.withData({
        'prometheus.yml': k.util.manifestYaml(prometheus_config),
      }),
    ]
    + (
      if std.prune(this.prometheusAlerts) != {}
      then [
        configMap.new('%s-alerts' % _config.name) +
        configMap.withData({
          'alerts.rules': k.util.manifestYaml(this.prometheusAlerts),
        }),
      ]
      else []
    ) + (
      if std.prune(this.prometheusRules) != {}
      then [
        configMap.new('%s-recording' % _config.name) +
        configMap.withData({
          'recording.rules': k.util.manifestYaml(this.prometheusRules),
        }),
      ]
      else []
    ),

  local policyRule = k.rbac.v1.policyRule,

  prometheus_rbac:
    k.util.rbac(_config.name, [
      policyRule.withApiGroups([''])
      + policyRule.withResources([
        'nodes',
        'nodes/proxy',
        'services',
        'endpoints',
        'pods',
      ])
      + policyRule.withVerbs(['get', 'list', 'watch']),

      policyRule.withNonResourceUrls('/metrics')
      + policyRule.withVerbs(['get']),
    ]),

  local container = k.core.v1.container,

  prometheus_container::
    container.new('prometheus', self._images.prometheus)
    + container.withPorts([
      k.core.v1.containerPort.new('http-metrics', _config.prometheus_port),
    ])
    + container.withArgs(std.prune([
      '--config.file=' + _config.prometheus_config_file,
      '--web.listen-address=:%s' % _config.prometheus_port,
      '--web.external-url=%(prometheus_external_hostname)s%(prometheus_path)s' % _config,
      '--web.enable-admin-api',
      '--web.enable-lifecycle',
      '--web.route-prefix=%s' % _config.prometheus_web_route_prefix,
      '--storage.tsdb.path=/prometheus/data',
      '--storage.tsdb.wal-compression',
      (if std.length(_config.prometheus_enabled_features) != 0
       then '--enable-feature=%s' % std.join(',', _config.prometheus_enabled_features)
       else null),
    ]))
    + k.util.resourcesRequests(_config.prometheus_requests_cpu,
                               _config.prometheus_requests_memory)
    + k.util.resourcesLimits(_config.prometheus_limits_cpu,
                             _config.prometheus_limits_memory)
  ,

  prometheus_watch_container::
    container.new('watch', self._images.watch) +
    container.withArgs([
      '-v',
      '-t',
      '-p=' + _config.prometheus_config_dir,
      'curl',
      '-X',
      'POST',
      '--fail',
      '-o',
      '-',
      '-sS',
      'http://localhost:%(prometheus_port)s%(prometheus_web_route_prefix)s-/reload' % _config,
    ]),

  local pvc = k.core.v1.persistentVolumeClaim,

  prometheus_pvc::
    pvc.new('%s-data' % (_config.name))
    + pvc.mixin.spec.withAccessModes('ReadWriteOnce')
    + pvc.mixin.spec.resources.withRequests({ storage: '300Gi' })
  ,

  local statefulset = k.apps.v1.statefulSet,
  local volumeMount = k.core.v1.volumeMount,

  prometheus_config_mount::
    k.util.configVolumeMount('%s-config' % _config.name, _config.prometheus_config_dir)
    + (if std.prune(this.prometheusAlerts) != {}
       then k.util.configVolumeMount('%s-alerts' % _config.name, _config.prometheus_config_dir + '/alerts')
       else {})
    + (if std.prune(this.prometheusRules) != {}
       then k.util.configVolumeMount('%s-recording' % _config.name, _config.prometheus_config_dir + '/recording')
       else {})
  ,

  prometheus_statefulset:
    statefulset.new(_config.name, 1, [
      self.prometheus_container + container.withVolumeMountsMixin(
        volumeMount.new('%s-data' % _config.name, '/prometheus')
      ),
      self.prometheus_watch_container,
    ], self.prometheus_pvc)
    + self.prometheus_config_mount
    + statefulset.mixin.spec.withPodManagementPolicy('Parallel')
    + statefulset.mixin.spec.withServiceName('prometheus')
    + statefulset.mixin.spec.template.metadata.withAnnotations({
      'prometheus.io.path': '%smetrics' % _config.prometheus_web_route_prefix,
    })
    + statefulset.mixin.spec.template.spec.withServiceAccount(_config.name)
    + statefulset.mixin.spec.template.spec.securityContext.withFsGroup(2000)
    + statefulset.mixin.spec.template.spec.securityContext.withRunAsUser(1000)
    + statefulset.mixin.spec.template.spec.securityContext.withRunAsNonRoot(true)
    + k.util.podPriority('critical')
  ,

  local service = k.core.v1.service,
  local servicePort = k.core.v1.servicePort,

  prometheus_service:
    k.util.serviceFor(self.prometheus_statefulset)
    + service.mixin.spec.withPortsMixin([
      servicePort.newNamed(
        name='http',
        port=80,
        targetPort=_config.prometheus_port,
      ),
    ]),
}
