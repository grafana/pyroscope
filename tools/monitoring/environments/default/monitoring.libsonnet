local phlare = import './../../../../operations/phlare/jsonnet/phlare-mixin/phlare-mixin/mixin.libsonnet';
local grafana = import 'grafana/grafana.libsonnet';
local k = import 'ksonnet-util/kausal.libsonnet';
local prom_k_grafana = import 'prometheus-ksonnet/grafana/grafana.libsonnet';
local prometheus = import 'prometheus-ksonnet/prometheus-ksonnet.libsonnet';

local namespace = 'monitoring';
local fqdn = {
  phlare: 'phlare-micro-services-query-frontend.default.svc.cluster.local.',
};


prometheus {
  local cluster_name = 'dev',
  _config+:: {
    cluster_name: cluster_name,
    namespace: namespace,
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

  agent:
    local pvc = k.core.v1.persistentVolumeClaim;
    local volumeMount = k.core.v1.volumeMount;
    local containerPort = k.core.v1.containerPort;
    local extraPorts = [
      {
        name: 'thrift-compact',
        port: 6831,
        protocol: 'UDP',
      },
      {
        name: 'sampling',
        port: 5778,
        protocol: 'TCP',
      },
    ];
    local agent = import 'github.com/grafana/agent/production/tanka/grafana-agent/v2/main.libsonnet';
    agent.new(name='grafana-agent', namespace=$._config.namespace) +
    agent.withStatefulSetController(
      replicas=1,
    ) +
    // problems with remote sampling in anything later than v0.28.1
    // see
    // https://github.com/grafana/agent/issues/2911
    agent.withImagesMixin({ agent: 'grafana/agent:v0.28.1' }) +
    agent.withArgsMixin({
      'enable-features': 'integrations-next',
    },) +
    // add dummy config or else will fail
    agent.withAgentConfig({
      server: { log_level: 'debug' },
    }) +
    agent.withVolumeMountsMixin([volumeMount.new('agent-wal', '/var/lib/agent')]) +
    // headless svc needed by statefulset
    agent.withService() +
    {
      controller_service+: {
        spec+: {
          clusterIP: 'None',
        },
      },
      controller+: {
        spec+: {
          template+: { spec+: {
            volumes+:
              local volume = k.core.v1.volume;
              [
                volume.fromEmptyDir('agent-wal'),
              ],
          } },
        },
      },


      container+: {
        ports+: [
          std.prune(p {
            containerPort: p.port,
            port::: null,
          })
          for p in extraPorts
        ],
      },

      configMap+: {
        data+: {
          'strategy.json': std.manifestJsonMinified({
            default_strategy: { param: 1.0, type: 'probabilistic' },
          }),
        },
      },

      jaeger_service:
        local svc = k.core.v1.service;

        super.controller_service +
        svc.metadata.withName('jaeger') +
        svc.metadata.withLabelsMixin({ name: 'jaeger' }) +
        svc.spec.withPorts([
          p {
            targetPort: p.port,
          }
          for p in extraPorts
        ]),
    },

  grafana_datasource_config_map+: k.core.v1.configMap.withDataMixin({
    'phlare-datasource.yml': k.util.manifestYaml({
      apiVersion: 1,
      datasources: [
        prom_k_grafana.grafana_datasource(
          'Phlare',
          'http://%s:4100' % fqdn.phlare,
          type='phlare'
        ),
      ],
    }),
  }),
} +
(import 'tempo.libsonnet') +
(import 'loki.libsonnet')
