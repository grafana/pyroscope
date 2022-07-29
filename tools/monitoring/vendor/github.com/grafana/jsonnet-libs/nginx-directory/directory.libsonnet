local configfile = import 'configfile.libsonnet';
local kausal = import 'ksonnet-util/kausal.libsonnet';
local link_data = import 'link_data.libsonnet';

(import 'config.libsonnet')
+ {
  local this = self,
  local k = kausal { _config+:: this._config },

  withName(name):: {
    nginx_container+:
      k.core.v1.container.withName(name),
    nginx_deployment+:
      k.apps.v1.deployment.metadata.withName(name),
    nginx_config_map+:
      k.core.v1.configMap.metadata.withName('%s-config' % name),
    nginx_html_config_map+:
      k.core.v1.configMap.metadata.withName('%s-config-html' % name),
  },

  // set default values
  local services = [
    {
      params: '',
      redirect: false,
      allowWebsockets: false,
      subfilter: false,
      // subfilter_content_types configures the content types handled by the subfilter (nginx sub_filter_types directive).
      subfilter_content_types: ['text/css', 'application/xml', 'application/json', 'application/javascript'],
      custom: [],

      // backwards compatible, service level config allows for more granular configuration
      read_timeout: this._config.nginx_proxy_read_timeout,
      send_timeout: this._config.nginx_proxy_send_timeout,
    } + service
    for service in this._config.admin_services
  ],

  local configMap = k.core.v1.configMap,
  nginx_config_map:
    configMap.new('nginx-config') +
    configMap.withData({
      'nginx.conf': (importstr 'files/nginx.conf') % (this._config + configfile(services)),
    }),

  nginx_html_config_map:
    configMap.new('nginx-config-html') +
    configMap.withData({
      'index.html': (importstr 'files/index.html') % (this._config + link_data(services, this._config.nginx_directory_sorted)),
    }),

  local container = k.core.v1.container,
  nginx_container::
    container.new('nginx', this._images.nginx) +
    container.withPorts(k.core.v1.containerPort.new('http', 80)) +
    k.util.resourcesRequests('50m', '100Mi'),

  local deployment = k.apps.v1.deployment,
  nginx_deployment:
    deployment.new('nginx', 1, [this.nginx_container]) +
    k.util.configMapVolumeMount(this.nginx_config_map, '/etc/nginx') +
    k.util.configMapVolumeMount(this.nginx_html_config_map, '/var/www/html') +
    k.util.podPriority('critical'),

  nginx_service:
    k.util.serviceFor(this.nginx_deployment),

  withOAuth2Proxy(config, images={}):: {
    local oauth2_proxy = import 'oauth2-proxy/oauth2-proxy.libsonnet',

    oauth2_proxy:
      oauth2_proxy {
        _config+:: config {
          oauth_upstream: 'http://nginx.%(namespace)s.svc.%(cluster_dns_suffix)s/' % config,
        },
        _images+:: images,
      },
  },

  // backwards compatible
  oauth2_proxy:
    if this._config.oauth_enabled
    then this.withOAuth2Proxy(
      this._config,
      images=if std.objectHasAll(this, '_images') then this._images else {}
    ).oauth2_proxy
    else {},
}
