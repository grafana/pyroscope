{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='endpointPort', url='', help='"EndpointPort represents a Port used by an EndpointSlice"'),
  '#withAppProtocol':: d.fn(help='"The application protocol for this port. This field follows standard Kubernetes label syntax. Un-prefixed names are reserved for IANA standard service names (as per RFC-6335 and http://www.iana.org/assignments/service-names). Non-standard protocols should use prefixed names such as mycompany.com/my-custom-protocol."', args=[d.arg(name='appProtocol', type=d.T.string)]),
  withAppProtocol(appProtocol): { appProtocol: appProtocol },
  '#withName':: d.fn(help="\"The name of this port. All ports in an EndpointSlice must have a unique name. If the EndpointSlice is dervied from a Kubernetes service, this corresponds to the Service.ports[].name. Name must either be an empty string or pass DNS_LABEL validation: * must be no more than 63 characters long. * must consist of lower case alphanumeric characters or '-'. * must start and end with an alphanumeric character. Default is empty string.\"", args=[d.arg(name='name', type=d.T.string)]),
  withName(name): { name: name },
  '#withPort':: d.fn(help='"The port number of the endpoint. If this is not specified, ports are not restricted and must be interpreted in the context of the specific consumer."', args=[d.arg(name='port', type=d.T.integer)]),
  withPort(port): { port: port },
  '#withProtocol':: d.fn(help='"The IP protocol for this port. Must be UDP, TCP, or SCTP. Default is TCP."', args=[d.arg(name='protocol', type=d.T.string)]),
  withProtocol(protocol): { protocol: protocol },
  '#mixin': 'ignore',
  mixin: self,
}
