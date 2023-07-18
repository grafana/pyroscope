{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='overhead', url='', help='"Overhead structure represents the resource overhead associated with running a pod."'),
  '#withPodFixed':: d.fn(help='"PodFixed represents the fixed resource overhead associated with running a pod."', args=[d.arg(name='podFixed', type=d.T.object)]),
  withPodFixed(podFixed): { podFixed: podFixed },
  '#withPodFixedMixin':: d.fn(help='"PodFixed represents the fixed resource overhead associated with running a pod."\n\n**Note:** This function appends passed data to existing values', args=[d.arg(name='podFixed', type=d.T.object)]),
  withPodFixedMixin(podFixed): { podFixed+: podFixed },
  '#mixin': 'ignore',
  mixin: self,
}
