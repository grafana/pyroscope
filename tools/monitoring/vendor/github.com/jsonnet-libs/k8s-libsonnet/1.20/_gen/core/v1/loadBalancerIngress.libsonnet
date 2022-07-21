{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='loadBalancerIngress', url='', help='"LoadBalancerIngress represents the status of a load-balancer ingress point: traffic intended for the service should be sent to an ingress point."'),
  '#withHostname':: d.fn(help='"Hostname is set for load-balancer ingress points that are DNS based (typically AWS load-balancers)"', args=[d.arg(name='hostname', type=d.T.string)]),
  withHostname(hostname): { hostname: hostname },
  '#withIp':: d.fn(help='"IP is set for load-balancer ingress points that are IP based (typically GCE or OpenStack load-balancers)"', args=[d.arg(name='ip', type=d.T.string)]),
  withIp(ip): { ip: ip },
  '#withPorts':: d.fn(help='"Ports is a list of records of service ports If used, every port defined in the service should have an entry in it"', args=[d.arg(name='ports', type=d.T.array)]),
  withPorts(ports): { ports: if std.isArray(v=ports) then ports else [ports] },
  '#withPortsMixin':: d.fn(help='"Ports is a list of records of service ports If used, every port defined in the service should have an entry in it"\n\n**Note:** This function appends passed data to existing values', args=[d.arg(name='ports', type=d.T.array)]),
  withPortsMixin(ports): { ports+: if std.isArray(v=ports) then ports else [ports] },
  '#mixin': 'ignore',
  mixin: self,
}
