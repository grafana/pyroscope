local k = import 'github.com/grafana/jsonnet-libs/ksonnet-util/kausal.libsonnet';

{
  new(image='prom/node-exporter:v1.3.1'):: {
    ignored_fs_types:: [
      'tmpfs',
      'autofs',
      'binfmt_misc',
      'bpf',
      'cgroup2?',
      'configfs',
      'debugfs',
      'devpts',
      'devtmpfs',
      'fusectl',
      'hugetlbfs',
      'mqueue',
      'nsfs',
      'overlay',
      'proc',
      'procfs',
      'pstore',
      'rpc_pipefs',
      'securityfs',
      'selinuxfs',
      'squashfs',
      'sysfs',
      'tracefs',
    ],

    local container = k.core.v1.container,
    container::
      container.new('node-exporter', image)
      + container.withPorts(
        k.core.v1.containerPort.new('http-metrics', 9100)
      )
      + container.withArgs([
        '--path.procfs=/host/proc',
        '--path.sysfs=/host/sys',

        // Reduces cardinality by ignoring a few devices, fs-types and mount-points.
        '--collector.netdev.device-exclude=^veth.+$',
        '--collector.filesystem.fs-types-exclude=^(%s)$' % std.join('|', self.ignored_fs_types),
        '--collector.filesystem.mount-points-exclude=^/(rootfs/)?(dev|proc|sys|var/lib/docker/.+)($|/)',
      ])
      + container.mixin.securityContext.withPrivileged(true)
      + container.mixin.securityContext.withRunAsUser(0)
      + k.util.resourcesRequests('50m', '30Mi')
      + k.util.resourcesLimits('200m', '75Mi'),

    local daemonSet = k.apps.v1.daemonSet,
    daemonset:
      daemonSet.new('node-exporter', [self.container])
      + daemonSet.mixin.spec.template.spec.withHostPid(true)
      + daemonSet.mixin.spec.template.spec.withHostNetwork(true)
      + k.util.hostVolumeMount('proc', '/proc', '/host/proc')
      + k.util.hostVolumeMount('sys', '/sys', '/host/sys')
      // Prevent default pod discovery from scraping, use ./scrape_config.libsonnet instead
      // to map the nodename to instance label.
      + daemonSet.mixin.spec.template.metadata.withAnnotationsMixin({ 'prometheus.io.scrape': 'false' }),
  },

  mountRoot():: {
    daemonset+: k.util.hostVolumeMount('root', '/', '/rootfs'),
  },

  scrape_config: (import './scrape_config.libsonnet'),
}
