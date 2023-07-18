{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='groupSubject', url='', help='"GroupSubject holds detailed information for group-kind subject."'),
  '#withName':: d.fn(help='"name is the user group that matches, or \\"*\\" to match all user groups. See https://github.com/kubernetes/apiserver/blob/master/pkg/authentication/user/user.go for some well-known group names. Required."', args=[d.arg(name='name', type=d.T.string)]),
  withName(name): { name: name },
  '#mixin': 'ignore',
  mixin: self,
}
