{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='ingressClassSpec', url='', help='"IngressClassSpec provides information about the class of an Ingress."'),
  '#parameters':: d.obj(help='"TypedLocalObjectReference contains enough information to let you locate the typed referenced object inside the same namespace."'),
  parameters: {
    '#withApiGroup':: d.fn(help='"APIGroup is the group for the resource being referenced. If APIGroup is not specified, the specified Kind must be in the core API group. For any other third-party types, APIGroup is required."', args=[d.arg(name='apiGroup', type=d.T.string)]),
    withApiGroup(apiGroup): { parameters+: { apiGroup: apiGroup } },
    '#withKind':: d.fn(help='"Kind is the type of resource being referenced"', args=[d.arg(name='kind', type=d.T.string)]),
    withKind(kind): { parameters+: { kind: kind } },
    '#withName':: d.fn(help='"Name is the name of resource being referenced"', args=[d.arg(name='name', type=d.T.string)]),
    withName(name): { parameters+: { name: name } },
  },
  '#withController':: d.fn(help='"Controller refers to the name of the controller that should handle this class. This allows for different \\"flavors\\" that are controlled by the same controller. For example, you may have different Parameters for the same implementing controller. This should be specified as a domain-prefixed path no more than 250 characters in length, e.g. \\"acme.io/ingress-controller\\". This field is immutable."', args=[d.arg(name='controller', type=d.T.string)]),
  withController(controller): { controller: controller },
  '#mixin': 'ignore',
  mixin: self,
}
