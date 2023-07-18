{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='boundObjectReference', url='', help='"BoundObjectReference is a reference to an object that a token is bound to."'),
  '#withKind':: d.fn(help="\"Kind of the referent. Valid kinds are 'Pod' and 'Secret'.\"", args=[d.arg(name='kind', type=d.T.string)]),
  withKind(kind): { kind: kind },
  '#withName':: d.fn(help='"Name of the referent."', args=[d.arg(name='name', type=d.T.string)]),
  withName(name): { name: name },
  '#withUid':: d.fn(help='"UID of the referent."', args=[d.arg(name='uid', type=d.T.string)]),
  withUid(uid): { uid: uid },
  '#mixin': 'ignore',
  mixin: self,
}
