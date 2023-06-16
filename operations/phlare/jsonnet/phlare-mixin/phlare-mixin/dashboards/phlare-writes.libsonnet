local g = import 'grafana-builder/grafana.libsonnet';
local utils = import 'mixin-utils/utils.libsonnet';


(import 'dashboard-utils.libsonnet') {
  grafanaDashboards+: {
    local dashboards = self,

    'phlare-writes.json': {
                            local cfg = self,

                            showMultiCluster:: $._config.multi_cluster,
                            clusterLabel:: $._config.per_cluster_label,
                            clusterMatchers::
                              if cfg.showMultiCluster then
                                [utils.selector.re(cfg.clusterLabel, '$cluster')]
                              else
                                [],

                            matchers:: {
                              distributor: [utils.selector.re('job', '($namespace)/(phlare|distributor)')],
                              ingester: [utils.selector.re('job', '($namespace)/(phlare|ingester)')],
                            },

                            local selector(matcherId) =
                              local ms = cfg.clusterMatchers + cfg.matchers[matcherId];
                              if std.length(ms) > 0 then
                                std.join(',', ['%(label)s%(op)s"%(value)s"' % matcher for matcher in ms]) + ','
                              else '',

                            distributorSelector:: selector('distributor'),
                            ingesterSelector:: selector('ingester'),
                          } +
                          $.dashboard('Phlare / Writes', uid='phlare-writes')
                          .addCluster()
                          .addNamespace()
                          .addTag()
                          .addRow(
                            $.row('Distributor Profiles received')
                            .addPanel(
                              $.panel('Compressed Size') +
                              utils.latencyRecordingRulePanel(
                                'pyroscope_distributor_received_compressed_bytes',
                                dashboards['phlare-writes.json'].matchers.distributor + [utils.selector.re('type', '.*')] + dashboards['phlare-writes.json'].clusterMatchers,
                                multiplier='1',
                                sum_by=['type'],
                              ) + { yaxes: g.yaxes('bytes') },
                            )
                            .addPanel(
                              $.panel('Samples') +
                              utils.latencyRecordingRulePanel(
                                'pyroscope_distributor_received_samples',
                                dashboards['phlare-writes.json'].matchers.distributor + [utils.selector.re('type', '.*')] + dashboards['phlare-writes.json'].clusterMatchers,
                                multiplier='1',
                                sum_by=['type'],
                              ) + { yaxes: g.yaxes('count') },
                            )
                          )
                          .addRow(
                            $.row('Distributor Requests')
                            .addPanel(
                              $.panel('QPS') +
                              $.qpsPanel('pyroscope_request_duration_seconds_count{%s, route=~".*push.*|.*ingest.*"}' % std.rstripChars(dashboards['phlare-writes.json'].distributorSelector, ','))
                            )
                            .addPanel(
                              $.panel('Latency') +
                              utils.latencyRecordingRulePanel(
                                'pyroscope_request_duration_seconds',
                                dashboards['phlare-writes.json'].matchers.distributor + [utils.selector.re('route', '.*push.*')] + dashboards['phlare-writes.json'].clusterMatchers,
                              )
                            )
                          )
                          .addRow(
                            $.row('Ingester')
                            .addPanel(
                              $.panel('QPS') +
                              $.qpsPanel('pyroscope_request_duration_seconds_count{%s route=~".*push.*|.*ingest.*"}' % dashboards['phlare-writes.json'].ingesterSelector)
                            )
                            .addPanel(
                              $.panel('Latency') +
                              utils.latencyRecordingRulePanel(
                                'pyroscope_request_duration_seconds',
                                dashboards['phlare-writes.json'].matchers.ingester + [utils.selector.re('route', '.*push.*|.*ingest.*')] + dashboards['phlare-writes.json'].clusterMatchers,
                              )
                            )
                          )
                          .addRow(
                            local long_desc = |||
                              Ingesters maintain a local Head per-tenant. Each
                              Head maintains the active profiling series; Then
                              the head gets periodically compacted into a block
                              on disk. This panel shows the estimated size of
                              the Head in memory for all ingesters.
                            |||;
                            $.row('Ingester - Head')
                            .addPanel(
                              local short_desc = 'Head size in bytes per table type';
                              $.panel(short_desc) +
                              $.panelDescription(short_desc, long_desc,) +
                              $.queryPanel('sum(pyroscope_head_size_bytes{%s}) by (type)' % dashboards['phlare-writes.json'].ingesterSelector, '{{type}}') +
                              { yaxes: $.yaxes('bytes') },
                            )
                            .addPanel(
                              local short_desc = 'Head size in bytes per pod';
                              $.panel(short_desc) +
                              $.panelDescription(short_desc, long_desc,) +
                              $.queryPanel('sum(pyroscope_head_size_bytes{%s}) by (instance)' % dashboards['phlare-writes.json'].ingesterSelector, '{{instance}}') +
                              { yaxes: $.yaxes('bytes') },
                            )
                          ),
  },
}
