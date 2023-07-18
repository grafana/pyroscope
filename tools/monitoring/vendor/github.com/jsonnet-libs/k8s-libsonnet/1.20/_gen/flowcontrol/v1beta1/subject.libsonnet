{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='subject', url='', help='"Subject matches the originator of a request, as identified by the request authentication system. There are three ways of matching an originator; by user, group, or service account."'),
  '#group':: d.obj(help='"GroupSubject holds detailed information for group-kind subject."'),
  group: {
    '#withName':: d.fn(help='"name is the user group that matches, or \\"*\\" to match all user groups. See https://github.com/kubernetes/apiserver/blob/master/pkg/authentication/user/user.go for some well-known group names. Required."', args=[d.arg(name='name', type=d.T.string)]),
    withName(name): { group+: { name: name } },
  },
  '#serviceAccount':: d.obj(help='"ServiceAccountSubject holds detailed information for service-account-kind subject."'),
  serviceAccount: {
    '#withName':: d.fn(help='"`name` is the name of matching ServiceAccount objects, or \\"*\\" to match regardless of name. Required."', args=[d.arg(name='name', type=d.T.string)]),
    withName(name): { serviceAccount+: { name: name } },
    '#withNamespace':: d.fn(help='"`namespace` is the namespace of matching ServiceAccount objects. Required."', args=[d.arg(name='namespace', type=d.T.string)]),
    withNamespace(namespace): { serviceAccount+: { namespace: namespace } },
  },
  '#user':: d.obj(help='"UserSubject holds detailed information for user-kind subject."'),
  user: {
    '#withName':: d.fn(help='"`name` is the username that matches, or \\"*\\" to match all usernames. Required."', args=[d.arg(name='name', type=d.T.string)]),
    withName(name): { user+: { name: name } },
  },
  '#withKind':: d.fn(help='"Required"', args=[d.arg(name='kind', type=d.T.string)]),
  withKind(kind): { kind: kind },
  '#mixin': 'ignore',
  mixin: self,
}
