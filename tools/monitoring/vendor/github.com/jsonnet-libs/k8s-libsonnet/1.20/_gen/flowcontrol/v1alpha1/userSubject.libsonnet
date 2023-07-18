{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='userSubject', url='', help='"UserSubject holds detailed information for user-kind subject."'),
  '#withName':: d.fn(help='"`name` is the username that matches, or \\"*\\" to match all usernames. Required."', args=[d.arg(name='name', type=d.T.string)]),
  withName(name): { name: name },
  '#mixin': 'ignore',
  mixin: self,
}
