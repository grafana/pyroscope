local grafana = import 'grafana/grafana.libsonnet';
local k = import 'ksonnet-util/kausal.libsonnet';
local datasource = grafana.datasource;
{
  local configMap = k.core.v1.configMap,

  /*
    to add datasources:

    grafanaDatasources+:: {
      'my-datasource.yml': $.grafana_datasource(name, url, default, method),
      'secure-datasource.yml': $.grafana_datasource_with_basicauth(name, url, username, password, default, method),
    },
  */
  grafanaDatasources+:: {},

  // Generates yaml string containing datasource config
  grafana_datasource(name, url, default=false, method='GET', type='prometheus')::
    datasource.new(name, url, type, default)
    + datasource.withHttpMethod(method)
  ,
  /*
    helper to allow adding datasources directly to the datasource_config_map
    eg:

    grafana_datasource_config_map+:
      $.grafana_add_datasource(name, url, default, method),
  */
  grafana_add_datasource(name, url, default=false, method='GET')::
    configMap.withDataMixin({
      ['%s.yml' % name]: k.util.manifestYaml({
        apiVersion: 1,
        datasources: [$.grafana_datasource(name, url, default, method)],
      }),
    }),

  // Generates yaml string containing datasource config
  grafana_datasource_with_basicauth(name, url, username, password, default=false, method='GET', type='prometheus')::
    self.datasource.new(name, url, type, default)
    + self.datasource.withHttpMethod(method)
    + self.datasource.withBasicAuth(username, password)
  ,

  /*
   helper to allow adding datasources directly to the datasource_config_map
   eg:

   grafana_datasource_config_map+:
     $.grafana_add_datasource_with_basicauth(name, url, username, password, default, method),
  */
  grafana_add_datasource_with_basicauth(name, url, username, password, default=false, method='GET', type='prometheus')::
    configMap.withDataMixin({
      ['%s.yml' % name]: k.util.manifestYaml({
        apiVersion: 1,
        datasources: [$.grafana_datasource_with_basicauth(name, url, username, password, default, method, type)],
      }),
    }),

  grafana_datasource_config_map:
    configMap.new('grafana-datasources') +
    configMap.withDataMixin({
      [if std.endsWith(name, '.yml') then name else name + '.yml']: (
        if std.isString($.grafanaDatasources[name]) then
          $.grafanaDatasources[name]
        else
          k.util.manifestYaml({
            apiVersion: 1,
            datasources: [$.grafanaDatasources[name]],
          })
      )
      for name in std.objectFields($.grafanaDatasources)
    }) +
    configMap.mixin.metadata.withLabels($._config.grafana_datasource_labels),
}
