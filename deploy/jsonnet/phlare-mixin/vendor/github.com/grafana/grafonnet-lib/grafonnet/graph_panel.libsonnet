{
  /**
   * Creates a [graph panel](https://grafana.com/docs/grafana/latest/panels/visualizations/graph-panel/).
   * It requires the graph panel plugin in grafana, which is built-in.
   *
   * @name graphPanel.new
   *
   * @param title The title of the graph panel.
   * @param description (optional) The description of the panel
   * @param span (optional) Width of the panel
   * @param datasource (optional) Datasource
   * @param fill (default `1`) , integer from 0 to 10
   * @param fillGradient (default `0`) , integer from 0 to 10
   * @param linewidth (default `1`) Line Width, integer from 0 to 10
   * @param decimals (optional) Override automatic decimal precision for legend and tooltip. If null, not added to the json output.
   * @param decimalsY1 (optional) Override automatic decimal precision for the first Y axis. If null, use decimals parameter.
   * @param decimalsY2 (optional) Override automatic decimal precision for the second Y axis. If null, use decimals parameter.
   * @param min_span (optional) Min span
   * @param format (default `short`) Unit of the Y axes
   * @param formatY1 (optional) Unit of the first Y axis
   * @param formatY2 (optional) Unit of the second Y axis
   * @param min (optional) Min of the Y axes
   * @param max (optional) Max of the Y axes
   * @param maxDataPoints (optional) If the data source supports it, sets the maximum number of data points for each series returned.
   * @param labelY1 (optional) Label of the first Y axis
   * @param labelY2 (optional) Label of the second Y axis
   * @param x_axis_mode (default `'time'`) X axis mode, one of [time, series, histogram]
   * @param x_axis_values (default `'total'`) Chosen value of series, one of [avg, min, max, total, count]
   * @param x_axis_buckets (optional) Restricts the x axis to this amount of buckets
   * @param x_axis_min (optional) Restricts the x axis to display from this value if supplied
   * @param x_axis_max (optional) Restricts the x axis to display up to this value if supplied
   * @param lines (default `true`) Display lines
   * @param points (default `false`) Display points
   * @param pointradius (default `5`) Radius of the points, allowed values are 0.5 or [1 ... 10] with step 1
   * @param bars (default `false`) Display bars
   * @param staircase (default `false`) Display line as staircase
   * @param dashes (default `false`) Display line as dashes
   * @param stack (default `false`) Whether to stack values
   * @param repeat (optional) Name of variable that should be used to repeat this panel.
   * @param repeatDirection (default `'h'`) 'h' for horizontal or 'v' for vertical.
   * @param legend_show (default `true`) Show legend
   * @param legend_values (default `false`) Show values in legend
   * @param legend_min (default `false`) Show min in legend
   * @param legend_max (default `false`) Show max in legend
   * @param legend_current (default `false`) Show current in legend
   * @param legend_total (default `false`) Show total in legend
   * @param legend_avg (default `false`) Show average in legend
   * @param legend_alignAsTable (default `false`) Show legend as table
   * @param legend_rightSide (default `false`) Show legend to the right
   * @param legend_sideWidth (optional) Legend width
   * @param legend_sort (optional) Sort order of legend
   * @param legend_sortDesc (optional) Sort legend descending
   * @param aliasColors (optional) Define color mappings for graphs
   * @param thresholds (optional) An array of graph thresholds
   * @param logBase1Y (default `1`) Value of logarithm base of the first Y axis
   * @param logBase2Y (default `1`) Value of logarithm base of the second Y axis
   * @param transparent (default `false`) Whether to display the panel without a background.
   * @param value_type (default `'individual'`) Type of tooltip value
   * @param shared_tooltip (default `true`) Allow to group or spit tooltips on mouseover within a chart
   * @param percentage (defaut: false) show as percentages
   * @param interval (defaut: null) A lower limit for the interval.

   *
   * @method addTarget(target) Adds a target object.
   * @method addTargets(targets) Adds an array of targets.
   * @method addSeriesOverride(override)
   * @method addYaxis(format,min,max,label,show,logBase,decimals) Adds a Y axis to the graph
   * @method addAlert(alert) Adds an alert
   * @method addLink(link) Adds a [panel link](https://grafana.com/docs/grafana/latest/linking/panel-links/)
   * @method addLinks(links) Adds an array of links.
   */
  new(
    title,
    span=null,
    fill=1,
    fillGradient=0,
    linewidth=1,
    decimals=null,
    decimalsY1=null,
    decimalsY2=null,
    description=null,
    min_span=null,
    format='short',
    formatY1=null,
    formatY2=null,
    min=null,
    max=null,
    labelY1=null,
    labelY2=null,
    x_axis_mode='time',
    x_axis_values='total',
    x_axis_buckets=null,
    x_axis_min=null,
    x_axis_max=null,
    lines=true,
    datasource=null,
    points=false,
    pointradius=5,
    bars=false,
    staircase=false,
    height=null,
    nullPointMode='null',
    dashes=false,
    stack=false,
    repeat=null,
    repeatDirection=null,
    sort=0,
    show_xaxis=true,
    legend_show=true,
    legend_values=false,
    legend_min=false,
    legend_max=false,
    legend_current=false,
    legend_total=false,
    legend_avg=false,
    legend_alignAsTable=false,
    legend_rightSide=false,
    legend_sideWidth=null,
    legend_hideEmpty=null,
    legend_hideZero=null,
    legend_sort=null,
    legend_sortDesc=null,
    aliasColors={},
    thresholds=[],
    links=[],
    logBase1Y=1,
    logBase2Y=1,
    transparent=false,
    value_type='individual',
    shared_tooltip=true,
    percentage=false,
    maxDataPoints=null,
    time_from=null,
    time_shift=null,
    interval=null
  ):: {
    title: title,
    [if span != null then 'span']: span,
    [if min_span != null then 'minSpan']: min_span,
    [if decimals != null then 'decimals']: decimals,
    type: 'graph',
    datasource: datasource,
    targets: [
    ],
    [if description != null then 'description']: description,
    [if height != null then 'height']: height,
    renderer: 'flot',
    yaxes: [
      self.yaxe(
        if formatY1 != null then formatY1 else format,
        min,
        max,
        decimals=(if decimalsY1 != null then decimalsY1 else decimals),
        logBase=logBase1Y,
        label=labelY1
      ),
      self.yaxe(
        if formatY2 != null then formatY2 else format,
        min,
        max,
        decimals=(if decimalsY2 != null then decimalsY2 else decimals),
        logBase=logBase2Y,
        label=labelY2
      ),
    ],
    xaxis: {
      show: show_xaxis,
      mode: x_axis_mode,
      name: null,
      values: if x_axis_mode == 'series' then [x_axis_values] else [],
      buckets: if x_axis_mode == 'histogram' then x_axis_buckets else null,
      [if x_axis_min != null then 'min']: x_axis_min,
      [if x_axis_max != null then 'max']: x_axis_max,
    },
    lines: lines,
    fill: fill,
    fillGradient: fillGradient,
    linewidth: linewidth,
    dashes: dashes,
    dashLength: 10,
    spaceLength: 10,
    points: points,
    pointradius: pointradius,
    bars: bars,
    stack: stack,
    percentage: percentage,
    [if maxDataPoints != null then 'maxDataPoints']: maxDataPoints,
    legend: {
      show: legend_show,
      values: legend_values,
      min: legend_min,
      max: legend_max,
      current: legend_current,
      total: legend_total,
      alignAsTable: legend_alignAsTable,
      rightSide: legend_rightSide,
      sideWidth: legend_sideWidth,
      avg: legend_avg,
      [if legend_hideEmpty != null then 'hideEmpty']: legend_hideEmpty,
      [if legend_hideZero != null then 'hideZero']: legend_hideZero,
      [if legend_sort != null then 'sort']: legend_sort,
      [if legend_sortDesc != null then 'sortDesc']: legend_sortDesc,
    },
    nullPointMode: nullPointMode,
    steppedLine: staircase,
    tooltip: {
      value_type: value_type,
      shared: shared_tooltip,
      sort: if sort == 'decreasing' then 2 else if sort == 'increasing' then 1 else sort,
    },
    timeFrom: time_from,
    timeShift: time_shift,
    [if interval != null then 'interval']: interval,
    [if transparent == true then 'transparent']: transparent,
    aliasColors: aliasColors,
    repeat: repeat,
    [if repeatDirection != null then 'repeatDirection']: repeatDirection,
    seriesOverrides: [],
    thresholds: thresholds,
    links: links,
    yaxe(
      format='short',
      min=null,
      max=null,
      label=null,
      show=true,
      logBase=1,
      decimals=null,
    ):: {
      label: label,
      show: show,
      logBase: logBase,
      min: min,
      max: max,
      format: format,
      [if decimals != null then 'decimals']: decimals,
    },
    _nextTarget:: 0,
    addTarget(target):: self {
      // automatically ref id in added targets.
      // https://github.com/kausalco/public/blob/master/klumps/grafana.libsonnet
      local nextTarget = super._nextTarget,
      _nextTarget: nextTarget + 1,
      targets+: [target { refId: std.char(std.codepoint('A') + nextTarget) }],
    },
    addTargets(targets):: std.foldl(function(p, t) p.addTarget(t), targets, self),
    addSeriesOverride(override):: self {
      seriesOverrides+: [override],
    },
    resetYaxes():: self {
      yaxes: [],
    },
    addYaxis(
      format='short',
      min=null,
      max=null,
      label=null,
      show=true,
      logBase=1,
      decimals=null,
    ):: self {
      yaxes+: [self.yaxe(format, min, max, label, show, logBase, decimals)],
    },
    addAlert(
      name,
      executionErrorState='alerting',
      forDuration='5m',
      frequency='60s',
      handler=1,
      message='',
      noDataState='no_data',
      notifications=[],
      alertRuleTags={},
    ):: self {
      local it = self,
      _conditions:: [],
      alert: {
        name: name,
        conditions: it._conditions,
        executionErrorState: executionErrorState,
        'for': forDuration,
        frequency: frequency,
        handler: handler,
        noDataState: noDataState,
        notifications: notifications,
        message: message,
        alertRuleTags: alertRuleTags,
      },
      addCondition(condition):: self {
        _conditions+: [condition],
      },
      addConditions(conditions):: std.foldl(function(p, c) p.addCondition(c), conditions, it),
    },
    addLink(link):: self {
      links+: [link],
    },
    addLinks(links):: std.foldl(function(p, t) p.addLink(t), links, self),
    addOverride(
      matcher=null,
      properties=null,
    ):: self {
      fieldConfig+: {
        overrides+: [
          {
            [if matcher != null then 'matcher']: matcher,
            [if properties != null then 'properties']: properties,
          },
        ],
      },
    },
    addOverrides(overrides):: std.foldl(function(p, o) p.addOverride(o.matcher, o.properties), overrides, self),
  },
}
