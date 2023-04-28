local utils = import 'mixin-utils/utils.libsonnet';

(import 'dashboard-utils.libsonnet') {
  grafanaDashboards+: {
    local dashboards = self,
    local http_route = '.*merge.*|.*series.*|.*type.*',
    'phlare-reads.json': {
                           local cfg = self,

                           showMultiCluster:: $._config.multi_cluster,
                           clusterLabel:: $._config.per_cluster_label,
                           clusterMatchers::
                             if cfg.showMultiCluster then
                               [utils.selector.re(cfg.clusterLabel, '$cluster')]
                             else
                               [],

                           matchers:: {
                             querier: [utils.selector.re('job', '($namespace)/(phlare|querier)')],
                             ingester: [utils.selector.re('job', '($namespace)/(phlare|ingester)')],
                           },

                           local selector(matcherId) =
                             local ms = (cfg.clusterMatchers + cfg.matchers[matcherId]);
                             if std.length(ms) > 0 then
                               std.join(',', ['%(label)s%(op)s"%(value)s"' % matcher for matcher in ms]) + ','
                             else '',
                           querierSelector:: selector('querier'),
                           ingesterSelector:: selector('ingester'),
                         } +
                         $.dashboard('Phlare / Reads', uid='phlare-reads')
                         .addCluster()
                         .addNamespace()
                         .addTag()
                         .addRow(
                           $.row('Querier')
                           .addPanel(
                             $.panel('QPS') +
                             $.qpsPanel('phlare_request_duration_seconds_count{%s route=~"%s"}' % [dashboards['phlare-reads.json'].querierSelector, http_route])
                           )
                           .addPanel(
                             $.panel('Latency') +
                             utils.latencyRecordingRulePanel(
                               'phlare_request_duration_seconds',
                               dashboards['phlare-reads.json'].matchers.querier + [utils.selector.re('route', http_route)] + dashboards['phlare-reads.json'].clusterMatchers,
                               sum_by=['route']
                             )
                           )
                         )
                         .addRow(
                           $.row('Ingester')
                           .addPanel(
                             $.panel('QPS') +
                             $.qpsPanel('phlare_request_duration_seconds_count{%s route=~"%s"}' % [dashboards['phlare-reads.json'].ingesterSelector, http_route])
                           )
                           .addPanel(
                             $.panel('Latency') +
                             utils.latencyRecordingRulePanel(
                               'phlare_request_duration_seconds',
                               dashboards['phlare-reads.json'].matchers.ingester + [utils.selector.re('route', http_route)] + dashboards['phlare-reads.json'].clusterMatchers,
                               sum_by=['route']
                             )
                           )
                         ),
  },
}
