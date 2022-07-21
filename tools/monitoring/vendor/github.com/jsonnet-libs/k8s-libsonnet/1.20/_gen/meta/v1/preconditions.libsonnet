{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='preconditions', url='', help='"Preconditions must be fulfilled before an operation (update, delete, etc.) is carried out."'),
  '#withResourceVersion':: d.fn(help='"Specifies the target ResourceVersion"', args=[d.arg(name='resourceVersion', type=d.T.string)]),
  withResourceVersion(resourceVersion): { resourceVersion: resourceVersion },
  '#withUid':: d.fn(help='"Specifies the target UID."', args=[d.arg(name='uid', type=d.T.string)]),
  withUid(uid): { uid: uid },
  '#mixin': 'ignore',
  mixin: self,
}
