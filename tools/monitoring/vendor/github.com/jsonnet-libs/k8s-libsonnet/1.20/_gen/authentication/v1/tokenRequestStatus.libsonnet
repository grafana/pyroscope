{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='tokenRequestStatus', url='', help='"TokenRequestStatus is the result of a token request."'),
  '#withExpirationTimestamp':: d.fn(help='"Time is a wrapper around time.Time which supports correct marshaling to YAML and JSON.  Wrappers are provided for many of the factory methods that the time package offers."', args=[d.arg(name='expirationTimestamp', type=d.T.string)]),
  withExpirationTimestamp(expirationTimestamp): { expirationTimestamp: expirationTimestamp },
  '#withToken':: d.fn(help='"Token is the opaque bearer token."', args=[d.arg(name='token', type=d.T.string)]),
  withToken(token): { token: token },
  '#mixin': 'ignore',
  mixin: self,
}
