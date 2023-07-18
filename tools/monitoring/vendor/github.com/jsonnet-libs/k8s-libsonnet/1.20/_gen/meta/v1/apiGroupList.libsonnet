{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='apiGroupList', url='', help='"APIGroupList is a list of APIGroup, to allow clients to discover the API at /apis."'),
  '#new':: d.fn(help='new returns an instance of APIGroupList', args=[d.arg(name='name', type=d.T.string)]),
  new(name): {
    apiVersion: 'v1',
    kind: 'APIGroupList',
  } + self.metadata.withName(name=name),
  '#withGroups':: d.fn(help='"groups is a list of APIGroup."', args=[d.arg(name='groups', type=d.T.array)]),
  withGroups(groups): { groups: if std.isArray(v=groups) then groups else [groups] },
  '#withGroupsMixin':: d.fn(help='"groups is a list of APIGroup."\n\n**Note:** This function appends passed data to existing values', args=[d.arg(name='groups', type=d.T.array)]),
  withGroupsMixin(groups): { groups+: if std.isArray(v=groups) then groups else [groups] },
  '#mixin': 'ignore',
  mixin: self,
}
