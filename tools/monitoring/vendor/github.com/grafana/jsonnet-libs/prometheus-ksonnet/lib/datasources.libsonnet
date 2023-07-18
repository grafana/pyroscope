{
  grafanaDatasources+:: {
    'prometheus.yml': $.grafana_datasource('prometheus',
                                           'http://prometheus.%(prometheus_namespace)s.svc.%(cluster_dns_suffix)s%(prometheus_web_route_prefix)s' % $._config,
                                           default=true),
  },
}
