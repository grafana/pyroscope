{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='volumeNodeResources', url='', help='"VolumeNodeResources is a set of resource limits for scheduling of volumes."'),
  '#withCount':: d.fn(help='"Maximum number of unique volumes managed by the CSI driver that can be used on a node. A volume that is both attached and mounted on a node is considered to be used once, not twice. The same rule applies for a unique volume that is shared among multiple pods on the same node. If this field is nil, then the supported number of volumes on this node is unbounded."', args=[d.arg(name='count', type=d.T.integer)]),
  withCount(count): { count: count },
  '#mixin': 'ignore',
  mixin: self,
}
