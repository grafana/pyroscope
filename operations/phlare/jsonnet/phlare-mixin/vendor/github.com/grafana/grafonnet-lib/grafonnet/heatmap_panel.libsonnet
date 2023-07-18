{
  /**
   * Creates a [heatmap panel](https://grafana.com/docs/grafana/latest/panels/visualizations/heatmap/).
   * Requires the heatmap panel plugin in Grafana, which is built-in.
   *
   * @name heatmapPanel.new
   *
   * @param title The title of the heatmap panel
   * @param description (optional) Description of panel
   * @param datasource (optional) Datasource
   * @param min_span (optional) Min span
   * @param span (optional) Width of the panel
   * @param cards_cardPadding (optional) How much padding to put between bucket cards
   * @param cards_cardRound (optional) How much rounding should be applied to the bucket card shape
   * @param color_cardColor (default `'#b4ff00'`) Hex value of color used when color_colorScheme is 'opacity'
   * @param color_colorScale (default `'sqrt'`) How to scale the color range, 'linear' or 'sqrt'
   * @param color_colorScheme (default `'interpolateOranges'`) TODO: document
   * @param color_exponent (default `0.5`) TODO: document
   * @param color_max (optional) The value for the end of the color range
   * @param color_min (optional) The value for the beginning of the color range
   * @param color_mode (default `'spectrum'`) How to display difference in frequency with color
   * @param dataFormat (default `'timeseries'`) How to format the data
   * @param highlightCards (default `true`) TODO: document
   * @param hideZeroBuckets (default `false`) Whether or not to hide empty buckets, default is false
   * @param legend_show (default `false`) Show legend
   * @param minSpan (optional) Minimum span of the panel when repeated on a template variable
   * @param repeat (optional) Variable used to repeat the heatmap panel
   * @param repeatDirection (optional) Which direction to repeat the panel, 'h' for horizontal and 'v' for vertically
   * @param tooltipDecimals (optional) The number of decimal places to display in the tooltip
   * @param tooltip_show (default `true`) Whether or not to display a tooltip when hovering over the heatmap
   * @param tooltip_showHistogram (default `false`) Whether or not to display a histogram in the tooltip
   * @param xAxis_show (default `true`) Whether or not to show the X axis, default true
   * @param xBucketNumber (optional) Number of buckets for the X axis
   * @param xBucketSize (optional) Size of X axis buckets. Number or interval(10s, 15h, etc.) Has priority over xBucketNumber
   * @param yAxis_decimals (optional) Override automatic decimal precision for the Y axis
   * @param yAxis_format (default `'short'`) Unit of the Y axis
   * @param yAxis_logBase (default `1`) Only if dataFormat is 'timeseries'
   * @param yAxis_min (optional) Only if dataFormat is 'timeseries', min of the Y axis
   * @param yAxis_max (optional) Only if dataFormat is 'timeseries', max of the Y axis
   * @param yAxis_show (default `true`) Whether or not to show the Y axis
   * @param yAxis_splitFactor (optional) TODO: document
   * @param yBucketBound (default `'auto'`) Which bound ('lower' or 'upper') of the bucket to use
   * @param yBucketNumber (optional) Number of buckets for the Y axis
   * @param yBucketSize (optional) Size of Y axis buckets. Has priority over yBucketNumber
   * @param maxDataPoints (optional) The maximum data points per series. Used directly by some data sources and used in calculation of auto interval. With streaming data this value is used for the rolling buffer.
   *
   * @method addTarget(target) Adds a target object.
   * @method addTargets(targets) Adds an array of targets.
   */
  new(
    title,
    datasource=null,
    description=null,
    cards_cardPadding=null,
    cards_cardRound=null,
    color_cardColor='#b4ff00',
    color_colorScale='sqrt',
    color_colorScheme='interpolateOranges',
    color_exponent=0.5,
    color_max=null,
    color_min=null,
    color_mode='spectrum',
    dataFormat='timeseries',
    highlightCards=true,
    hideZeroBuckets=false,
    legend_show=false,
    minSpan=null,
    span=null,
    repeat=null,
    repeatDirection=null,
    tooltipDecimals=null,
    tooltip_show=true,
    tooltip_showHistogram=false,
    xAxis_show=true,
    xBucketNumber=null,
    xBucketSize=null,
    yAxis_decimals=null,
    yAxis_format='short',
    yAxis_logBase=1,
    yAxis_min=null,
    yAxis_max=null,
    yAxis_show=true,
    yAxis_splitFactor=null,
    yBucketBound='auto',
    yBucketNumber=null,
    yBucketSize=null,
    maxDataPoints=null,
  ):: {
    title: title,
    type: 'heatmap',
    [if description != null then 'description']: description,
    datasource: datasource,
    cards: {
      cardPadding: cards_cardPadding,
      cardRound: cards_cardRound,
    },
    color: {
      mode: color_mode,
      cardColor: color_cardColor,
      colorScale: color_colorScale,
      exponent: color_exponent,
      [if color_mode == 'spectrum' then 'colorScheme']: color_colorScheme,
      [if color_max != null then 'max']: color_max,
      [if color_min != null then 'min']: color_min,
    },
    [if dataFormat != null then 'dataFormat']: dataFormat,
    heatmap: {},
    hideZeroBuckets: hideZeroBuckets,
    highlightCards: highlightCards,
    legend: {
      show: legend_show,
    },
    [if minSpan != null then 'minSpan']: minSpan,
    [if span != null then 'span']: span,
    [if repeat != null then 'repeat']: repeat,
    [if repeatDirection != null then 'repeatDirection']: repeatDirection,
    tooltip: {
      show: tooltip_show,
      showHistogram: tooltip_showHistogram,
    },
    [if tooltipDecimals != null then 'tooltipDecimals']: tooltipDecimals,
    xAxis: {
      show: xAxis_show,
    },
    xBucketNumber: if dataFormat == 'timeseries' && xBucketSize != null then xBucketNumber else null,
    xBucketSize: if dataFormat == 'timeseries' && xBucketSize != null then xBucketSize else null,
    yAxis: {
      decimals: yAxis_decimals,
      [if dataFormat == 'timeseries' then 'logBase']: yAxis_logBase,
      format: yAxis_format,
      [if dataFormat == 'timeseries' then 'max']: yAxis_max,
      [if dataFormat == 'timeseries' then 'min']: yAxis_min,
      show: yAxis_show,
      splitFactor: yAxis_splitFactor,
    },
    yBucketBound: yBucketBound,
    [if dataFormat == 'timeseries' then 'yBucketNumber']: yBucketNumber,
    [if dataFormat == 'timeseries' then 'yBucketSize']: yBucketSize,
    [if maxDataPoints != null then 'maxDataPoints']: maxDataPoints,

    _nextTarget:: 0,
    addTarget(target):: self {
      local nextTarget = super._nextTarget,
      _nextTarget: nextTarget + 1,
      targets+: [target { refId: std.char(std.codepoint('A') + nextTarget) }],
    },
    addTargets(targets):: std.foldl(function(p, t) p.addTarget(t), targets, self),
  },

}
