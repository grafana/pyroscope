{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='ingressServiceBackend', url='', help='"IngressServiceBackend references a Kubernetes Service as a Backend."'),
  '#port':: d.obj(help='"ServiceBackendPort is the service port being referenced."'),
  port: {
    '#withName':: d.fn(help='"Name is the name of the port on the Service. This is a mutually exclusive setting with \\"Number\\"."', args=[d.arg(name='name', type=d.T.string)]),
    withName(name): { port+: { name: name } },
    '#withNumber':: d.fn(help='"Number is the numerical port number (e.g. 80) on the Service. This is a mutually exclusive setting with \\"Name\\"."', args=[d.arg(name='number', type=d.T.integer)]),
    withNumber(number): { port+: { number: number } },
  },
  '#withName':: d.fn(help='"Name is the referenced service. The service must exist in the same namespace as the Ingress object."', args=[d.arg(name='name', type=d.T.string)]),
  withName(name): { name: name },
  '#mixin': 'ignore',
  mixin: self,
}
