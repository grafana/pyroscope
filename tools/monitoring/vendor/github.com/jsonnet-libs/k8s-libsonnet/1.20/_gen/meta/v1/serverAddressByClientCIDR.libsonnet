{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='serverAddressByClientCIDR', url='', help='"ServerAddressByClientCIDR helps the client to determine the server address that they should use, depending on the clientCIDR that they match."'),
  '#withClientCIDR':: d.fn(help='"The CIDR with which clients can match their IP to figure out the server address that they should use."', args=[d.arg(name='clientCIDR', type=d.T.string)]),
  withClientCIDR(clientCIDR): { clientCIDR: clientCIDR },
  '#withServerAddress':: d.fn(help='"Address of this server, suitable for a client that matches the above CIDR. This can be a hostname, hostname:port, IP or IP:port."', args=[d.arg(name='serverAddress', type=d.T.string)]),
  withServerAddress(serverAddress): { serverAddress: serverAddress },
  '#mixin': 'ignore',
  mixin: self,
}
