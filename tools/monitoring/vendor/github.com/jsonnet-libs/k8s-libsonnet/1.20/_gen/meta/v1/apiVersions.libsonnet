{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='apiVersions', url='', help='"APIVersions lists the versions that are available, to allow clients to discover the API at /api, which is the root path of the legacy v1 API."'),
  '#new':: d.fn(help='new returns an instance of APIVersions', args=[d.arg(name='name', type=d.T.string)]),
  new(name): {
    apiVersion: 'v1',
    kind: 'APIVersions',
  } + self.metadata.withName(name=name),
  '#withServerAddressByClientCIDRs':: d.fn(help='"a map of client CIDR to server address that is serving this group. This is to help clients reach servers in the most network-efficient way possible. Clients can use the appropriate server address as per the CIDR that they match. In case of multiple matches, clients should use the longest matching CIDR. The server returns only those CIDRs that it thinks that the client can match. For example: the master will return an internal IP CIDR only, if the client reaches the server using an internal IP. Server looks at X-Forwarded-For header or X-Real-Ip header or request.RemoteAddr (in that order) to get the client IP."', args=[d.arg(name='serverAddressByClientCIDRs', type=d.T.array)]),
  withServerAddressByClientCIDRs(serverAddressByClientCIDRs): { serverAddressByClientCIDRs: if std.isArray(v=serverAddressByClientCIDRs) then serverAddressByClientCIDRs else [serverAddressByClientCIDRs] },
  '#withServerAddressByClientCIDRsMixin':: d.fn(help='"a map of client CIDR to server address that is serving this group. This is to help clients reach servers in the most network-efficient way possible. Clients can use the appropriate server address as per the CIDR that they match. In case of multiple matches, clients should use the longest matching CIDR. The server returns only those CIDRs that it thinks that the client can match. For example: the master will return an internal IP CIDR only, if the client reaches the server using an internal IP. Server looks at X-Forwarded-For header or X-Real-Ip header or request.RemoteAddr (in that order) to get the client IP."\n\n**Note:** This function appends passed data to existing values', args=[d.arg(name='serverAddressByClientCIDRs', type=d.T.array)]),
  withServerAddressByClientCIDRsMixin(serverAddressByClientCIDRs): { serverAddressByClientCIDRs+: if std.isArray(v=serverAddressByClientCIDRs) then serverAddressByClientCIDRs else [serverAddressByClientCIDRs] },
  '#withVersions':: d.fn(help='"versions are the api versions that are available."', args=[d.arg(name='versions', type=d.T.array)]),
  withVersions(versions): { versions: if std.isArray(v=versions) then versions else [versions] },
  '#withVersionsMixin':: d.fn(help='"versions are the api versions that are available."\n\n**Note:** This function appends passed data to existing values', args=[d.arg(name='versions', type=d.T.array)]),
  withVersionsMixin(versions): { versions+: if std.isArray(v=versions) then versions else [versions] },
  '#mixin': 'ignore',
  mixin: self,
}
