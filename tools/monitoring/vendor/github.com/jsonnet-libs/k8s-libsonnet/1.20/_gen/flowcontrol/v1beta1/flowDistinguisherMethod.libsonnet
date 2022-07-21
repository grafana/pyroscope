{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='flowDistinguisherMethod', url='', help='"FlowDistinguisherMethod specifies the method of a flow distinguisher."'),
  '#withType':: d.fn(help='"`type` is the type of flow distinguisher method The supported types are \\"ByUser\\" and \\"ByNamespace\\". Required."', args=[d.arg(name='type', type=d.T.string)]),
  withType(type): { type: type },
  '#mixin': 'ignore',
  mixin: self,
}
