local g = import 'grafana-builder/grafana.libsonnet';

{
  histogramRules(metric, labels, interval='1m')::
    local vars = {
      metric: metric,
      labels_underscore: std.join('_', labels),
      labels_comma: std.join(', ', labels),
      interval: interval,
    };
    [
      {
        record: '%(labels_underscore)s:%(metric)s:99quantile' % vars,
        expr: 'histogram_quantile(0.99, sum(rate(%(metric)s_bucket[%(interval)s])) by (le, %(labels_comma)s))' % vars,
      },
      {
        record: '%(labels_underscore)s:%(metric)s:50quantile' % vars,
        expr: 'histogram_quantile(0.50, sum(rate(%(metric)s_bucket[%(interval)s])) by (le, %(labels_comma)s))' % vars,
      },
      {
        record: '%(labels_underscore)s:%(metric)s:avg' % vars,
        expr: 'sum(rate(%(metric)s_sum[1m])) by (%(labels_comma)s) / sum(rate(%(metric)s_count[%(interval)s])) by (%(labels_comma)s)' % vars,
      },
      {
        record: '%(labels_underscore)s:%(metric)s_bucket:sum_rate' % vars,
        expr: 'sum(rate(%(metric)s_bucket[%(interval)s])) by (le, %(labels_comma)s)' % vars,
      },
      {
        record: '%(labels_underscore)s:%(metric)s_sum:sum_rate' % vars,
        expr: 'sum(rate(%(metric)s_sum[%(interval)s])) by (%(labels_comma)s)' % vars,
      },
      {
        record: '%(labels_underscore)s:%(metric)s_count:sum_rate' % vars,
        expr: 'sum(rate(%(metric)s_count[%(interval)s])) by (%(labels_comma)s)' % vars,
      },
    ],


  // latencyRecordingRulePanel - build a latency panel for a recording rule.
  // - metric: the base metric name (middle part of recording rule name)
  // - selectors: list of selectors which will be added to first part of
  //   recording rule name, and to the query selector itself.
  // - extra_selectors (optional): list of selectors which will be added to the
  //   query selector, but not to the beginnig of the recording rule name.
  //   Useful for external labels.
  // - multiplier (optional): assumes results are in seconds, will multiply
  //   by 1e3 to get ms.  Can be turned off.
  // - sum_by (optional): additional labels to use in the sum by clause, will also be used in the legend
  latencyRecordingRulePanel(metric, selectors, extra_selectors=[], multiplier='1e3', sum_by=[])::
    local labels = std.join('_', [matcher.label for matcher in selectors]);
    local selectorStr = $.toPrometheusSelector(selectors + extra_selectors);
    local sb = ['le'];
    local legend = std.join('', ['{{ %(lb)s }} ' % lb for lb in sum_by]);
    local sumBy = if std.length(sum_by) > 0 then ' by (%(lbls)s) ' % { lbls: std.join(',', sum_by) } else '';
    local sumByHisto = std.join(',', sb + sum_by);
    {
      nullPointMode: 'null as zero',
      yaxes: g.yaxes('ms'),
      targets: [
        {
          expr: 'histogram_quantile(0.99, sum by (%(sumBy)s) (%(labels)s:%(metric)s_bucket:sum_rate%(selector)s)) * %(multiplier)s' % {
            labels: labels,
            metric: metric,
            selector: selectorStr,
            multiplier: multiplier,
            sumBy: sumByHisto,
          },
          format: 'time_series',
          intervalFactor: 2,
          legendFormat: '%(legend)s99th Percentile' % legend,
          refId: 'A',
          step: 10,
        },
        {
          expr: 'histogram_quantile(0.50, sum by (%(sumBy)s) (%(labels)s:%(metric)s_bucket:sum_rate%(selector)s)) * %(multiplier)s' % {
            labels: labels,
            metric: metric,
            selector: selectorStr,
            multiplier: multiplier,
            sumBy: sumByHisto,
          },
          format: 'time_series',
          intervalFactor: 2,
          legendFormat: '%(legend)s50th Percentile' % legend,
          refId: 'B',
          step: 10,
        },
        {
          expr: '%(multiplier)s * sum(%(labels)s:%(metric)s_sum:sum_rate%(selector)s)%(sumBy)s / sum(%(labels)s:%(metric)s_count:sum_rate%(selector)s)%(sumBy)s' % {
            labels: labels,
            metric: metric,
            selector: selectorStr,
            multiplier: multiplier,
            sumBy: sumBy,
          },
          format: 'time_series',
          intervalFactor: 2,
          legendFormat: '%(legend)sAverage' % legend,
          refId: 'C',
          step: 10,
        },
      ],
    },

  selector:: {
    eq(label, value):: { label: label, op: '=', value: value },
    neq(label, value):: { label: label, op: '!=', value: value },
    re(label, value):: { label: label, op: '=~', value: value },
    nre(label, value):: { label: label, op: '!~', value: value },

    // Use with latencyRecordingRulePanel to get the label in the metric name
    // but not in the selector.
    noop(label):: { label: label, op: 'nop' },
  },

  toPrometheusSelector(selector)::
    local pairs = [
      '%(label)s%(op)s"%(value)s"' % matcher
      for matcher in std.filter(function(matcher) matcher.op != 'nop', selector)
    ];
    '{%s}' % std.join(', ', pairs),

  // withRunbookURL - Add/Override the runbook_url annotations for all alerts inside a list of rule groups.
  // - url_format: an URL format for the runbook, the alert name will be substituted in the URL.
  // - groups: the list of rule groups containing alerts.
  withRunbookURL(url_format, groups)::
    local update_rule(rule) =
      if std.objectHas(rule, 'alert')
      then rule {
        annotations+: {
          runbook_url: url_format % rule.alert,
        },
      }
      else rule;
    [
      group {
        rules: [
          update_rule(alert)
          for alert in group.rules
        ],
      }
      for group in groups
    ],

  removeRuleGroup(ruleName):: {
    local removeRuleGroup(rule) = if rule.name == ruleName then null else rule,
    local currentRuleGroups = super.groups,
    groups: std.prune(std.map(removeRuleGroup, currentRuleGroups)),
  },

  removeAlertRuleGroup(ruleName):: {
    prometheusAlerts+:: $.removeRuleGroup(ruleName),
  },

  removeRecordingRuleGroup(ruleName):: {
    prometheusRules+:: $.removeRuleGroup(ruleName),
  },

  overrideAlerts(overrides):: {
    local overrideRule(rule) =
      if 'alert' in rule && std.objectHas(overrides, rule.alert)
      then rule + overrides[rule.alert]
      else rule,
    local overrideInGroup(group) = group { rules: std.map(overrideRule, super.rules) },
    prometheusAlerts+:: {
      groups: std.map(overrideInGroup, super.groups),
    },
  },
}
