{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='ingressBackend', url='', help='"IngressBackend describes all endpoints for a given service and port."'),
  '#resource':: d.obj(help='"TypedLocalObjectReference contains enough information to let you locate the typed referenced object inside the same namespace."'),
  resource: {
    '#withApiGroup':: d.fn(help='"APIGroup is the group for the resource being referenced. If APIGroup is not specified, the specified Kind must be in the core API group. For any other third-party types, APIGroup is required."', args=[d.arg(name='apiGroup', type=d.T.string)]),
    withApiGroup(apiGroup): { resource+: { apiGroup: apiGroup } },
    '#withKind':: d.fn(help='"Kind is the type of resource being referenced"', args=[d.arg(name='kind', type=d.T.string)]),
    withKind(kind): { resource+: { kind: kind } },
    '#withName':: d.fn(help='"Name is the name of resource being referenced"', args=[d.arg(name='name', type=d.T.string)]),
    withName(name): { resource+: { name: name } },
  },
  '#service':: d.obj(help='"IngressServiceBackend references a Kubernetes Service as a Backend."'),
  service: {
    '#port':: d.obj(help='"ServiceBackendPort is the service port being referenced."'),
    port: {
      '#withName':: d.fn(help='"Name is the name of the port on the Service. This is a mutually exclusive setting with \\"Number\\"."', args=[d.arg(name='name', type=d.T.string)]),
      withName(name): { service+: { port+: { name: name } } },
      '#withNumber':: d.fn(help='"Number is the numerical port number (e.g. 80) on the Service. This is a mutually exclusive setting with \\"Name\\"."', args=[d.arg(name='number', type=d.T.integer)]),
      withNumber(number): { service+: { port+: { number: number } } },
    },
    '#withName':: d.fn(help='"Name is the referenced service. The service must exist in the same namespace as the Ingress object."', args=[d.arg(name='name', type=d.T.string)]),
    withName(name): { service+: { name: name } },
  },
  '#mixin': 'ignore',
  mixin: self,
}
