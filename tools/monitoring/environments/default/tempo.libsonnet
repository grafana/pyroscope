local agent = import 'github.com/grafana/agent/production/tanka/grafana-agent/v2/main.libsonnet';
local tempo = import 'github.com/grafana/tempo/operations/jsonnet/single-binary/tempo.libsonnet';
local k = import 'ksonnet-util/kausal.libsonnet';
local prom_k_grafana = import 'prometheus-ksonnet/grafana/grafana.libsonnet';

{
  local namespace = if std.objectHas($._config, 'namespace') then $._config.namespace else 'mynamespace',

  _config+: {},

  agent+: {
    _config+: { agent_config+: {
      traces+: {
        configs: [{
          name: 'kubernetes-traces',
          receivers: {
            jaeger: {
              protocols: {
                grpc: null,
                thrift_binary: null,
                thrift_compact: null,
                thrift_http: null,
              },
              remote_sampling: {
                strategy_file: '/etc/agent/strategy.json',
                tls: {
                  insecure: true,
                },
              },
            },
          },
          remote_write: [{
            endpoint: '%s:4317' % $.tempo.fqdn,
            insecure: true,
            retry_on_failure: {
              enabled: true,
            },
          }],
          scrape_configs: agent.newKubernetesTraces({}),
        }],
      },
    } },
  },


  tempo:
    tempo {
      local cluster_name = 'dev',
      _config+:: {
        cluster_name: cluster_name,
        namespace: 'monitoring',
        receivers: {
          jaeger: {
            protocols: {
              grpc: null,
              thrift_http: null,
              thrift_binary: null,
              thrift_compact: null,
            },
          },
          zipkin: null,
          otlp: {
            protocols: {
              http: null,
              grpc: null,
            },
          },
          opencensus: null,
        },

        // need to set something, but will use empty dir anyhow
        pvc_size: null,
        pvc_storage_class: null,
      },

      tempo_container+:
        k.util.resourcesRequests('25m', '128Mi') +
        k.util.resourcesLimits(null, null),

      tempo_statefulset+: {
        spec+: {
          volumeClaimTemplates: [
          ],
          template+: { spec+: {
            volumes+:
              local volume = k.core.v1.volume;
              [
                volume.fromEmptyDir('tempo-data'),
              ],
          } },
        },
      },
    } {
      fqdn:: 'tempo.%s.svc.cluster.local.' % namespace,
    },

  grafana_datasource_config_map+: k.core.v1.configMap.withDataMixin({
    'tempo-datasource.yml': k.util.manifestYaml({
      apiVersion: 1,
      datasources: [
        prom_k_grafana.grafana_datasource(
          'Tempo',
          'http://%s:3200' % $.tempo.fqdn,
          type='tempo'
        ),
      ],
    }),
  }),
}
