{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='apiResourceList', url='', help='"APIResourceList is a list of APIResource, it is used to expose the name of the resources supported in a specific group and version, and if the resource is namespaced."'),
  '#new':: d.fn(help='new returns an instance of APIResourceList', args=[d.arg(name='name', type=d.T.string)]),
  new(name): {
    apiVersion: 'v1',
    kind: 'APIResourceList',
  } + self.metadata.withName(name=name),
  '#withGroupVersion':: d.fn(help='"groupVersion is the group and version this APIResourceList is for."', args=[d.arg(name='groupVersion', type=d.T.string)]),
  withGroupVersion(groupVersion): { groupVersion: groupVersion },
  '#withResources':: d.fn(help='"resources contains the name of the resources and if they are namespaced."', args=[d.arg(name='resources', type=d.T.array)]),
  withResources(resources): { resources: if std.isArray(v=resources) then resources else [resources] },
  '#withResourcesMixin':: d.fn(help='"resources contains the name of the resources and if they are namespaced."\n\n**Note:** This function appends passed data to existing values', args=[d.arg(name='resources', type=d.T.array)]),
  withResourcesMixin(resources): { resources+: if std.isArray(v=resources) then resources else [resources] },
  '#mixin': 'ignore',
  mixin: self,
}
