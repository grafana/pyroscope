local fire = import './../../../../deploy/jsonnet/fire-mixin/mixin.libsonnet';
local prometheus = import 'prometheus-ksonnet/prometheus-ksonnet.libsonnet';
local k = import 'ksonnet-util/kausal.libsonnet';
local prom_k_grafana = import 'prometheus-ksonnet/grafana/grafana.libsonnet';
local grafana = import 'grafana/grafana.libsonnet';


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
    fire: fire {
      grafanaPlugins: ['pyroscope-datasource', 'pyroscope-panel'],
    },
  },
  grafana_datasource_config_map+: k.core.v1.configMap.withDataMixin({
    'fire-datasource.yml': k.util.manifestYaml({
      apiVersion: 1,
      datasources: [
        prom_k_grafana.grafana_datasource(
          'fire',
          'http://fire-micro-services-querier.default.svc.cluster.local.:4100/pyroscope/',
          type='pyroscope-datasource'
        ) + grafana.datasource.withJsonData({
          path: 'http://fire-micro-services-querier.default.svc.cluster.local.:4100/pyroscope/',
        },),
      ],
    }),
  }),
}
