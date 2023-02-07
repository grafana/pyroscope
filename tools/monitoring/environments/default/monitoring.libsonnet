local phlare = import './../../../../operations/phlare/jsonnet/phlare-mixin/phlare-mixin/mixin.libsonnet';
local grafana = import 'grafana/grafana.libsonnet';
local k = import 'ksonnet-util/kausal.libsonnet';
local prom_k_grafana = import 'prometheus-ksonnet/grafana/grafana.libsonnet';
local prometheus = import 'prometheus-ksonnet/prometheus-ksonnet.libsonnet';


prometheus {
  local cluster_name = 'dev',
  _config+:: {
    cluster_name: cluster_name,
    namespace: 'monitoring',
    grafana_ini+: {
      sections+: {
        feature_toggles+: {
          enable: 'flameGraph',
        },
      },
    },
  },
  _images+:: {
    grafana: 'grafana/grafana:main',
  },
  prometheus+::
    prometheus.withMixinsConfigmaps($.mixins) + {
      name: 'prometheus',
      prometheus_config+:: {
        scrape_configs: [
          s {
            relabel_configs+: [
              {
                target_label: 'cluster',
                replacement: cluster_name,
              },

            ],
          }
          for s in super.scrape_configs
        ],
      },
    },
  mixins+:: {
    phlare: phlare {},
  },
  grafana_datasource_config_map+: k.core.v1.configMap.withDataMixin({
    'phlare-datasource.yml': k.util.manifestYaml({
      apiVersion: 1,
      datasources: [
        prom_k_grafana.grafana_datasource(
          'phlare',
          'http://phlare-micro-services-query-frontend.default.svc.cluster.local.:4100',
          type='phlare'
        ) + grafana.datasource.withJsonData({
          path: 'http://phlare-micro-services-query-frontend.default.svc.cluster.local.:4100/',
        },),
      ],
    }),
  }),
}
