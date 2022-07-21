local k = import 'ksonnet-util/kausal.libsonnet';

{
  _config+:: {
    oauth_cookie_secret: error 'Must define a cookie secret',
    oauth_client_id: error 'Must define a client id',
    oauth_client_secret: error 'Must define a client secret',
    oauth_redirect_url: error 'Must define a redirect url',
    oauth_upstream: error 'Must define an upstream',
    oauth_email_domain: '*',
    oauth_pass_basic_auth: 'false',
    oauth_extra_args: [],
  },

  _images+:: {
    oauth2_proxy: 'quay.io/oauth2-proxy/oauth2-proxy:v7.2.1',
  },

  local secret = k.core.v1.secret,

  oauth2_proxy_secret:
    secret.new('oauth2-proxy', {
      OAUTH2_PROXY_COOKIE_SECRET: std.base64($._config.oauth_cookie_secret),
      OAUTH2_PROXY_CLIENT_SECRET: std.base64($._config.oauth_client_secret),
      OAUTH2_PROXY_CLIENT_ID: std.base64($._config.oauth_client_id),
    }),

  local container = k.core.v1.container,
  local containerPort = k.core.v1.containerPort,
  local envFrom = k.core.v1.envFromSource,

  oauth2_proxy_container::
    container.new('oauth2-proxy', $._images.oauth2_proxy) +
    container.withPorts(containerPort.new('http', 4180)) +
    container.withArgs([
      '--http-address=0.0.0.0:4180',
      '--redirect-url=%s' % $._config.oauth_redirect_url,
      '--upstream=%s' % $._config.oauth_upstream,
      '--email-domain=%s' % $._config.oauth_email_domain,
      '--pass-basic-auth=%s' % $._config.oauth_pass_basic_auth,
    ] + $._config.oauth_extra_args) +
    container.withEnvFrom(
      envFrom.secretRef.withName('oauth2-proxy'),
    ),

  local deployment = k.apps.v1.deployment,

  oauth2_proxy_deployment:
    deployment.new('oauth2-proxy', 1, [$.oauth2_proxy_container]),

  oauth2_proxy_service:
    k.util.serviceFor($.oauth2_proxy_deployment),
}
