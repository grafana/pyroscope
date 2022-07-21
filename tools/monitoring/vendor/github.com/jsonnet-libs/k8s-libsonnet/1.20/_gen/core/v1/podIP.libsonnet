{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='podIP', url='', help='"IP address information for entries in the (plural) PodIPs field. Each entry includes:\\n   IP: An IP address allocated to the pod. Routable at least within the cluster."'),
  '#withIp':: d.fn(help='"ip is an IP address (IPv4 or IPv6) assigned to the pod"', args=[d.arg(name='ip', type=d.T.string)]),
  withIp(ip): { ip: ip },
  '#mixin': 'ignore',
  mixin: self,
}
