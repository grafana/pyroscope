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
  '#withServiceName':: d.fn(help='"Specifies the name of the referenced service."', args=[d.arg(name='serviceName', type=d.T.string)]),
  withServiceName(serviceName): { serviceName: serviceName },
  '#withServicePort':: d.fn(help='"IntOrString is a type that can hold an int32 or a string.  When used in JSON or YAML marshalling and unmarshalling, it produces or consumes the inner type.  This allows you to have, for example, a JSON field that can accept a name or number."', args=[d.arg(name='servicePort', type=d.T.string)]),
  withServicePort(servicePort): { servicePort: servicePort },
  '#mixin': 'ignore',
  mixin: self,
}
