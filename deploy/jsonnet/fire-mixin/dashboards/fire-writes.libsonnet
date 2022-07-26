local g = import 'grafana-builder/grafana.libsonnet';
local utils = import 'mixin-utils/utils.libsonnet';


(import 'dashboard-utils.libsonnet') {
  grafanaDashboards+: {
    local dashboards = self,

    'fire-writes.json': {
                          local cfg = self,

                          showMultiCluster:: $._config.multi_cluster,
                          clusterLabel:: $._config.per_cluster_label,
                          clusterMatchers::
                            if cfg.showMultiCluster then
                              [utils.selector.re(cfg.clusterLabel, '$cluster')]
                            else
                              [],

                          matchers:: {
                            distributor: [utils.selector.re('job', '($namespace)/distributor')],
                            ingester: [utils.selector.re('job', '($namespace)/ingester')],
                          },

                          local selector(matcherId) =
                            local ms = cfg.clusterMatchers + cfg.matchers[matcherId];
                            if std.length(ms) > 0 then
                              std.join(',', ['%(label)s%(op)s"%(value)s"' % matcher for matcher in ms]) + ','
                            else '',

                          distributorSelector:: selector('distributor'),
                          ingesterSelector:: selector('ingester'),
                        } +
                        $.dashboard('Fire / Writes', uid='writes')
                        .addCluster()
                        .addNamespace()
                        .addTag()
                        .addRow(
                          $.row('Distributor Profiles')
                          .addPanel(
                            $.panel('Compressed Size') +
                            utils.latencyRecordingRulePanel(
                              'fire_distributor_received_compressed_bytes',
                              dashboards['fire-writes.json'].matchers.distributor + [utils.selector.re('type', '.*')] + dashboards['fire-writes.json'].clusterMatchers,
                              multiplier='1',
                              sum_by=['type'],
                            ) + { yaxes: g.yaxes('bytes') },
                          )
                          .addPanel(
                            $.panel('Samples') +
                            utils.latencyRecordingRulePanel(
                              'fire_distributor_received_samples',
                              dashboards['fire-writes.json'].matchers.distributor + [utils.selector.re('type', '.*')] + dashboards['fire-writes.json'].clusterMatchers,
                              multiplier='1',
                              sum_by=['type'],
                            ) + { yaxes: g.yaxes('count') },
                          )
                        )
                        .addRow(
                          $.row('Distributor Requests')
                          .addPanel(
                            $.panel('QPS') +
                            $.qpsPanel('fire_request_duration_seconds_count{%s, route=~".*push.*"}' % std.rstripChars(dashboards['fire-writes.json'].distributorSelector, ','))
                          )
                          .addPanel(
                            $.panel('Latency') +
                            utils.latencyRecordingRulePanel(
                              'fire_request_duration_seconds',
                              dashboards['fire-writes.json'].matchers.distributor + [utils.selector.re('route', '.*push.*')] + dashboards['fire-writes.json'].clusterMatchers,
                            )
                          )
                        )
                        .addRow(
                          $.row('Ingester')
                          .addPanel(
                            $.panel('QPS') +
                            $.qpsPanel('fire_request_duration_seconds_count{%s route=~".*push.*"}' % dashboards['fire-writes.json'].ingesterSelector)
                          )
                          .addPanel(
                            $.panel('Latency') +
                            utils.latencyRecordingRulePanel(
                              'fire_request_duration_seconds',
                              dashboards['fire-writes.json'].matchers.ingester + [utils.selector.re('route', '.*push.*')] + dashboards['fire-writes.json'].clusterMatchers,
                            )
                          )
                        ),
  },
}
