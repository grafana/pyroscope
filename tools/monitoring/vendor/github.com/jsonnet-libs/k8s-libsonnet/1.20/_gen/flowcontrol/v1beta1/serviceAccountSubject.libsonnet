{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='serviceAccountSubject', url='', help='"ServiceAccountSubject holds detailed information for service-account-kind subject."'),
  '#withName':: d.fn(help='"`name` is the name of matching ServiceAccount objects, or \\"*\\" to match regardless of name. Required."', args=[d.arg(name='name', type=d.T.string)]),
  withName(name): { name: name },
  '#withNamespace':: d.fn(help='"`namespace` is the namespace of matching ServiceAccount objects. Required."', args=[d.arg(name='namespace', type=d.T.string)]),
  withNamespace(namespace): { namespace: namespace },
  '#mixin': 'ignore',
  mixin: self,
}
