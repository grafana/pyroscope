local phlare = import './../../../../deploy/jsonnet/phlare-mixin/mixin.libsonnet';
local grafana = import 'grafana/grafana.libsonnet';
local k = import 'ksonnet-util/kausal.libsonnet';
local prom_k_grafana = import 'prometheus-ksonnet/grafana/grafana.libsonnet';
local prometheus = import 'prometheus-ksonnet/prometheus-ksonnet.libsonnet';


prometheus {
  local cluster_name = 'dev',
  _config+:: {
    cluster_name: cluster_name,
    namespace: 'monitoring',
  },
  _images+:: {
    grafana: 'ctovena/grafana:hackathon-1',
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
    phlare: phlare {
      grafanaPlugins: ['pyroscope-datasource', 'pyroscope-panel'],
    },
  },
  grafana_datasource_config_map+: k.core.v1.configMap.withDataMixin({
    'phlare-datasource.yml': k.util.manifestYaml({
      apiVersion: 1,
      datasources: [
        prom_k_grafana.grafana_datasource(
          'phlare',
          'http://phlare-micro-services-querier.default.svc.cluster.local.:4100/pyroscope/',
          type='pyroscope-datasource'
        ) + grafana.datasource.withJsonData({
          path: 'http://phlare-micro-services-querier.default.svc.cluster.local.:4100/pyroscope/',
        },),
      ],
    }),
  }),
}
