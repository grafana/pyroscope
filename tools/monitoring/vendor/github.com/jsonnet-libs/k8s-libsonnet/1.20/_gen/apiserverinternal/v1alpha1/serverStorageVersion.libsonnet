{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='serverStorageVersion', url='', help='"An API server instance reports the version it can decode and the version it encodes objects to when persisting objects in the backend."'),
  '#withApiServerID':: d.fn(help='"The ID of the reporting API server."', args=[d.arg(name='apiServerID', type=d.T.string)]),
  withApiServerID(apiServerID): { apiServerID: apiServerID },
  '#withDecodableVersions':: d.fn(help='"The API server can decode objects encoded in these versions. The encodingVersion must be included in the decodableVersions."', args=[d.arg(name='decodableVersions', type=d.T.array)]),
  withDecodableVersions(decodableVersions): { decodableVersions: if std.isArray(v=decodableVersions) then decodableVersions else [decodableVersions] },
  '#withDecodableVersionsMixin':: d.fn(help='"The API server can decode objects encoded in these versions. The encodingVersion must be included in the decodableVersions."\n\n**Note:** This function appends passed data to existing values', args=[d.arg(name='decodableVersions', type=d.T.array)]),
  withDecodableVersionsMixin(decodableVersions): { decodableVersions+: if std.isArray(v=decodableVersions) then decodableVersions else [decodableVersions] },
  '#withEncodingVersion':: d.fn(help='"The API server encodes the object to this version when persisting it in the backend (e.g., etcd)."', args=[d.arg(name='encodingVersion', type=d.T.string)]),
  withEncodingVersion(encodingVersion): { encodingVersion: encodingVersion },
  '#mixin': 'ignore',
  mixin: self,
}
