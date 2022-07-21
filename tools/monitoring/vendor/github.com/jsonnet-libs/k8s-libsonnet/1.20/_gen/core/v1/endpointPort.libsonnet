{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='endpointPort', url='', help='"EndpointPort is a tuple that describes a single port."'),
  '#withAppProtocol':: d.fn(help='"The application protocol for this port. This field follows standard Kubernetes label syntax. Un-prefixed names are reserved for IANA standard service names (as per RFC-6335 and http://www.iana.org/assignments/service-names). Non-standard protocols should use prefixed names such as mycompany.com/my-custom-protocol. This is a beta field that is guarded by the ServiceAppProtocol feature gate and enabled by default."', args=[d.arg(name='appProtocol', type=d.T.string)]),
  withAppProtocol(appProtocol): { appProtocol: appProtocol },
  '#withName':: d.fn(help="\"The name of this port.  This must match the 'name' field in the corresponding ServicePort. Must be a DNS_LABEL. Optional only if one port is defined.\"", args=[d.arg(name='name', type=d.T.string)]),
  withName(name): { name: name },
  '#withPort':: d.fn(help='"The port number of the endpoint."', args=[d.arg(name='port', type=d.T.integer)]),
  withPort(port): { port: port },
  '#withProtocol':: d.fn(help='"The IP protocol for this port. Must be UDP, TCP, or SCTP. Default is TCP."', args=[d.arg(name='protocol', type=d.T.string)]),
  withProtocol(protocol): { protocol: protocol },
  '#mixin': 'ignore',
  mixin: self,
}
