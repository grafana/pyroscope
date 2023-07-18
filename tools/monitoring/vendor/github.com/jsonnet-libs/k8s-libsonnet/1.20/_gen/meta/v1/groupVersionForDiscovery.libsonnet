{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='groupVersionForDiscovery', url='', help='"GroupVersion contains the \\"group/version\\" and \\"version\\" string of a version. It is made a struct to keep extensibility."'),
  '#withGroupVersion':: d.fn(help='"groupVersion specifies the API group and version in the form \\"group/version\\', args=[d.arg(name='groupVersion', type=d.T.string)]),
  withGroupVersion(groupVersion): { groupVersion: groupVersion },
  '#withVersion':: d.fn(help='"version specifies the version in the form of \\"version\\". This is to save the clients the trouble of splitting the GroupVersion."', args=[d.arg(name='version', type=d.T.string)]),
  withVersion(version): { version: version },
  '#mixin': 'ignore',
  mixin: self,
}
