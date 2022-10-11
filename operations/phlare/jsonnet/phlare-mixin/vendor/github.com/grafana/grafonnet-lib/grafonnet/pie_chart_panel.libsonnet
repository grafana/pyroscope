{
  /**
   * Creates a pie chart panel.
   * It requires the [pie chart panel plugin in grafana](https://grafana.com/grafana/plugins/grafana-piechart-panel),
   * which needs to be explicitly installed.
   *
   * @name pieChartPanel.new
   *
   * @param title The title of the pie chart panel.
   * @param description (default `''`) Description of the panel
   * @param span (optional) Width of the panel
   * @param min_span (optional) Min span
   * @param datasource (optional) Datasource
   * @param aliasColors (optional) Define color mappings
   * @param pieType (default `'pie'`) Type of pie chart (one of pie or donut)
   * @param showLegend (default `true`) Show legend
   * @param showLegendPercentage (default `true`) Show percentage values in the legend
   * @param legendType (default `'Right side'`) Type of legend (one of 'Right side', 'Under graph' or 'On graph')
   * @param valueName (default `'current') Type of tooltip value
   * @param repeat (optional) Variable used to repeat the pie chart
   * @param repeatDirection (optional) Which direction to repeat the panel, 'h' for horizontal and 'v' for vertical
   * @param maxPerRow (optional) Number of panels to display when repeated. Used in combination with repeat.
   * @return A json that represents a pie chart panel
   *
   * @method addTarget(target) Adds a target object.
   */
  new(
    title,
    description='',
    span=null,
    min_span=null,
    datasource=null,
    height=null,
    aliasColors={},
    pieType='pie',
    valueName='current',
    showLegend=true,
    showLegendPercentage=true,
    legendType='Right side',
    repeat=null,
    repeatDirection=null,
    maxPerRow=null,
  ):: {
    type: 'grafana-piechart-panel',
    [if description != null then 'description']: description,
    pieType: pieType,
    title: title,
    aliasColors: aliasColors,
    [if span != null then 'span']: span,
    [if min_span != null then 'minSpan']: min_span,
    [if height != null then 'height']: height,
    [if repeat != null then 'repeat']: repeat,
    [if repeatDirection != null then 'repeatDirection']: repeatDirection,
    [if maxPerRow != null then 'maxPerRow']: maxPerRow,
    valueName: valueName,
    datasource: datasource,
    legend: {
      show: showLegend,
      values: true,
      percentage: showLegendPercentage,
    },
    legendType: legendType,
    targets: [
    ],
    _nextTarget:: 0,
    addTarget(target):: self {
      local nextTarget = super._nextTarget,
      _nextTarget: nextTarget + 1,
      targets+: [target { refId: std.char(std.codepoint('A') + nextTarget) }],
    },
  },
}
