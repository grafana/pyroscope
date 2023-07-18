local k = import 'ksonnet-util/kausal.libsonnet';

(import 'config.libsonnet')
+ {
  local _config = self._config,

  mixins+:: {
    base+: {
      prometheusAlerts+: {
        groups+: [
          {
            name: 'prometheus-extra',
            rules: [
              {
                alert: 'PromScrapeFailed',
                expr: |||
                  up != 1
                |||,
                'for': '15m',
                labels: {
                  severity: 'warning',
                },
                annotations: {
                  message: 'Prometheus failed to scrape a target {{ $labels.job }} / {{ $labels.instance }}',
                },
              },
              {
                alert: 'PromScrapeFlapping',
                expr: |||
                  avg_over_time(up[5m]) < 1
                |||,
                'for': '15m',
                labels: {
                  severity: 'warning',
                },
                annotations: {
                  message: 'Prometheus target flapping {{ $labels.job }} / {{ $labels.instance }}',
                },
              },
              {
                alert: 'PromScrapeTooLong',
                expr: |||
                  scrape_duration_seconds > 60
                |||,
                'for': '15m',
                labels: {
                  severity: 'warning',
                },
                annotations: {
                  message: '{{ $labels.job }} / {{ $labels.instance }} is taking too long to scrape ({{ printf "%.1f" $value }}s)',
                },
              },
            ],
          },
        ],
      },
    },
  },

  // Legacy Extension points for adding alerts, recording rules and prometheus config.
  local emptyMixin = {
    prometheusAlerts+:: {},
    prometheusRules+:: {},
  },

  withMixinsConfigmaps(mixins):: {
    local this = self,
    local configMap = k.core.v1.configMap,

    mixin_data:: [
      local mixin = mixins[mixinName] + emptyMixin;
      local prometheusAlerts = mixin.prometheusAlerts;
      local prometheusRules = mixin.prometheusRules;
      {
        mixinName: mixinName,
        configmapName: std.strReplace('%s-%s-mixin' % [_config.name, mixinName], '_', '-'),
        path: '%s/mixins/%s' % [_config.prometheus_config_dir, mixinName],
        files: {
          [if std.prune(prometheusAlerts) != {} then 'alerts.rules']: k.util.manifestYaml(prometheusAlerts),
          [if std.prune(prometheusRules) != {} then 'recording.rules']: k.util.manifestYaml(prometheusRules),
        },
        hasFiles: std.length(self.files) > 0,
      }
      for mixinName in std.objectFields(mixins)
    ],

    prometheus_config_maps_mixins+: [
      configMap.new(mixin.configmapName)
      + configMap.withData(mixin.files)
      for mixin in this.mixin_data
      if mixin.hasFiles
    ],

    prometheus_config+: {
      rule_files+: std.reverse(std.sort([
        '%s/%s' % [mixin.path, file]
        for mixin in this.mixin_data
        for file in std.objectFields(mixin.files)
      ])),
    },

    prometheus_config_mount+::
      std.foldr(
        function(mixin, acc)
          acc +
          if mixin.hasFiles
          then k.util.configVolumeMount(mixin.configmapName, mixin.path)
          else {},
        this.mixin_data,
        {}
      ),
  },

  // Extends legacy extension points with the mixins object, adding
  // the alerts and recording rules to root-level. This functions
  // should be applied if you want to retain the legacy behavior.
  withMixinsLegacyConfigmaps(mixins):: {
    prometheusAlerts+::
      std.foldr(
        function(mixinName, acc)
          local mixin = mixins[mixinName] + emptyMixin;
          acc + mixin.prometheusAlerts,
        std.objectFields(mixins),
        {}
      ),

    prometheusRules+::
      std.foldr(
        function(mixinName, acc)
          local mixin = mixins[mixinName] + emptyMixin;
          acc + mixin.prometheusRules,
        std.objectFields(mixins),
        {},
      ),
  },
}
