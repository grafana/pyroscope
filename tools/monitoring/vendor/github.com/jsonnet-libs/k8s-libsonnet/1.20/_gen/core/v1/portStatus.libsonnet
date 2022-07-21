{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='portStatus', url='', help=''),
  '#withError':: d.fn(help='"Error is to record the problem with the service port The format of the error shall comply with the following rules: - built-in error values shall be specified in this file and those shall use\\n  CamelCase names\\n- cloud provider specific error values must have names that comply with the\\n  format foo.example.com/CamelCase."', args=[d.arg(name='err', type=d.T.string)]),
  withError(err): { 'error': err },
  '#withPort':: d.fn(help='"Port is the port number of the service port of which status is recorded here"', args=[d.arg(name='port', type=d.T.integer)]),
  withPort(port): { port: port },
  '#withProtocol':: d.fn(help='"Protocol is the protocol of the service port of which status is recorded here The supported values are: \\"TCP\\", \\"UDP\\", \\"SCTP\\', args=[d.arg(name='protocol', type=d.T.string)]),
  withProtocol(protocol): { protocol: protocol },
  '#mixin': 'ignore',
  mixin: self,
}
