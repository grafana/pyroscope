{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='tokenRequest', url='', help='"TokenRequest contains parameters of a service account token."'),
  '#withAudience':: d.fn(help='"Audience is the intended audience of the token in \\"TokenRequestSpec\\". It will default to the audiences of kube apiserver."', args=[d.arg(name='audience', type=d.T.string)]),
  withAudience(audience): { audience: audience },
  '#withExpirationSeconds':: d.fn(help='"ExpirationSeconds is the duration of validity of the token in \\"TokenRequestSpec\\". It has the same default value of \\"ExpirationSeconds\\" in \\"TokenRequestSpec\\"."', args=[d.arg(name='expirationSeconds', type=d.T.integer)]),
  withExpirationSeconds(expirationSeconds): { expirationSeconds: expirationSeconds },
  '#mixin': 'ignore',
  mixin: self,
}
