local agent = import 'github.com/grafana/agent/production/tanka/grafana-agent/v2/main.libsonnet';
local loki = import 'github.com/grafana/loki/production/ksonnet/loki-simple-scalable/loki.libsonnet';
local k = import 'ksonnet-util/kausal.libsonnet';
local prom_k_grafana = import 'prometheus-ksonnet/grafana/grafana.libsonnet';


{
  local namespace = if std.objectHas($._config, 'namespace') then $._config.namespace else 'mynamespace',

  local podsLogPath = '/var/log/pods',

  _config+: {},

  agent+: {
    _config+: { agent_config+: {
      logs+: {
        configs: [{
          name: 'kubernetes-logs',
          clients: [{
            url: 'http://%s:3100/loki/api/v1/push' % $.loki.fqdn,
          }],
          positions: {
            filename: podsLogPath + '/grafana-agent-positions.yaml',
          },
          scrape_configs: agent.newKubernetesLogs({}),
        }],
      },
    } },

    controller+:
      local sts = k.apps.v1.statefulSet;
      sts.spec.template.spec.withVolumesMixin(
        local volume = k.core.v1.volume;
        [
          volume.fromHostPath('logs', podsLogPath),
        ]
      ),

    container+:
      {
        volumeMounts+:
          local volumeMount = k.core.v1.volumeMount;
          [
            volumeMount.new('logs', podsLogPath),
          ],

      },
  },

  loki:
    local upstream = loki {
      _config+:: {
        headless_service_name: 'loki',
        http_listen_port: 3100,
        read_replicas: 1,
        write_replicas: 1,
        loki: {
          auth_enabled: false,
        },
      },


      write_args: {
        'config.file': '/etc/loki/local-config.yaml',
      },

      write_statefulset+:
        {
          spec+: {
            volumeClaimTemplates:: [],
            template+: {
              spec+: {
                volumes:
                  local volume = k.core.v1.volume;
                  [
                    volume.fromEmptyDir('data'),
                  ],
                containers: [
                  c {
                    name: 'loki',
                    volumeMounts: [
                      x {
                        name: 'data',
                      }
                      for x in super.volumeMounts
                      if x.name == 'write-data'
                    ],
                  }
                  for c in super.containers
                ],
              },
            },
          },
        },
    };

    {
      fqdn:: 'loki.%s.svc.cluster.local.' % namespace,

      stateful_set:
        local sts = k.apps.v1.statefulSet;

        upstream.write_statefulset +
        sts.metadata.withName('loki') +
        sts.metadata.withLabelsMixin({ name: 'loki' }) +
        sts.spec.template.metadata.withLabelsMixin({ name: 'loki' }) +
        sts.spec.selector.withMatchLabelsMixin({ name: 'loki' }) +
        { spec+: { template+: { spec+: { affinity:: {} } } } },

      service:
        local svc = k.core.v1.service;
        svc.new('loki', { name: 'loki' }, [{ port: 3100 }]),

    },

  grafana_datasource_config_map+: k.core.v1.configMap.withDataMixin({
    'loki-datasource.yml': k.util.manifestYaml({
      apiVersion: 1,
      datasources: [
        prom_k_grafana.grafana_datasource(
          'Loki',
          'http://%s:3100' % $.loki.fqdn,
          type='loki'
        ),
      ],
    }),
  }),
}
