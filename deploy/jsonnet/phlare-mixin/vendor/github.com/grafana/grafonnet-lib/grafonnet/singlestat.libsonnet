{
  /**
   * Creates a singlestat panel.
   *
   * @name singlestat.new
   *
   * @param title The title of the singlestat panel.
   * @param format (default `'none'`) Unit
   * @param description (default `''`)
   * @param interval (optional)
   * @param height (optional)
   * @param datasource (optional)
   * @param span (optional)
   * @param min_span (optional)
   * @param decimals (optional)
   * @param valueName (default `'avg'`)
   * @param valueFontSize (default `'80%'`)
   * @param prefixFontSize (default `'50%'`)
   * @param postfixFontSize (default `'50%'`)
   * @param mappingType (default `1`)
   * @param repeat (optional)
   * @param repeatDirection (optional)
   * @param prefix (default `''`)
   * @param postfix (default `''`)
   * @param colors (default `['#299c46','rgba(237, 129, 40, 0.89)','#d44a3a']`)
   * @param colorBackground (default `false`)
   * @param colorValue (default `false`)
   * @param thresholds (default `''`)
   * @param valueMaps (default `{value: 'null',op: '=',text: 'N/A'}`)
   * @param rangeMaps (default `{value: 'null',op: '=',text: 'N/A'}`)
   * @param transparent (optional)
   * @param sparklineFillColor (default `'rgba(31, 118, 189, 0.18)'`)
   * @param sparklineFull (default `false`)
   * @param sparklineLineColor (default `'rgb(31, 120, 193)'`)
   * @param sparklineShow (default `false`)
   * @param gaugeShow (default `false`)
   * @param gaugeMinValue (default `0`)
   * @param gaugeMaxValue (default `100`)
   * @param gaugeThresholdMarkers (default `true`)
   * @param gaugeThresholdLabels (default `false`)
   * @param timeFrom (optional)
   * @param links (optional)
   * @param tableColumn (default `''`)
   * @param maxPerRow (optional)
   * @param maxDataPoints (default `100`)
   *
   * @method addTarget(target) Adds a target object.
   */
  new(
    title,
    format='none',
    description='',
    interval=null,
    height=null,
    datasource=null,
    span=null,
    min_span=null,
    decimals=null,
    valueName='avg',
    valueFontSize='80%',
    prefixFontSize='50%',
    postfixFontSize='50%',
    mappingType=1,
    repeat=null,
    repeatDirection=null,
    prefix='',
    postfix='',
    colors=[
      '#299c46',
      'rgba(237, 129, 40, 0.89)',
      '#d44a3a',
    ],
    colorBackground=false,
    colorValue=false,
    thresholds='',
    valueMaps=[
      {
        value: 'null',
        op: '=',
        text: 'N/A',
      },
    ],
    rangeMaps=[
      {
        from: 'null',
        to: 'null',
        text: 'N/A',
      },
    ],
    transparent=null,
    sparklineFillColor='rgba(31, 118, 189, 0.18)',
    sparklineFull=false,
    sparklineLineColor='rgb(31, 120, 193)',
    sparklineShow=false,
    gaugeShow=false,
    gaugeMinValue=0,
    gaugeMaxValue=100,
    gaugeThresholdMarkers=true,
    gaugeThresholdLabels=false,
    timeFrom=null,
    links=[],
    tableColumn='',
    maxPerRow=null,
    maxDataPoints=100,
  )::
    {
      [if height != null then 'height']: height,
      [if description != '' then 'description']: description,
      [if repeat != null then 'repeat']: repeat,
      [if repeatDirection != null then 'repeatDirection']: repeatDirection,
      [if transparent != null then 'transparent']: transparent,
      [if min_span != null then 'minSpan']: min_span,
      title: title,
      [if span != null then 'span']: span,
      type: 'singlestat',
      datasource: datasource,
      targets: [
      ],
      links: links,
      [if decimals != null then 'decimals']: decimals,
      maxDataPoints: maxDataPoints,
      interval: interval,
      cacheTimeout: null,
      format: format,
      prefix: prefix,
      postfix: postfix,
      nullText: null,
      valueMaps: valueMaps,
      [if maxPerRow != null then 'maxPerRow']: maxPerRow,
      mappingTypes: [
        {
          name: 'value to text',
          value: 1,
        },
        {
          name: 'range to text',
          value: 2,
        },
      ],
      rangeMaps: rangeMaps,
      mappingType:
        if mappingType == 'value'
        then
          1
        else if mappingType == 'range'
        then
          2
        else
          mappingType,
      nullPointMode: 'connected',
      valueName: valueName,
      prefixFontSize: prefixFontSize,
      valueFontSize: valueFontSize,
      postfixFontSize: postfixFontSize,
      thresholds: thresholds,
      [if timeFrom != null then 'timeFrom']: timeFrom,
      colorBackground: colorBackground,
      colorValue: colorValue,
      colors: colors,
      gauge: {
        show: gaugeShow,
        minValue: gaugeMinValue,
        maxValue: gaugeMaxValue,
        thresholdMarkers: gaugeThresholdMarkers,
        thresholdLabels: gaugeThresholdLabels,
      },
      sparkline: {
        fillColor: sparklineFillColor,
        full: sparklineFull,
        lineColor: sparklineLineColor,
        show: sparklineShow,
      },
      tableColumn: tableColumn,
      _nextTarget:: 0,
      addTarget(target):: self {
        local nextTarget = super._nextTarget,
        _nextTarget: nextTarget + 1,
        targets+: [target { refId: std.char(std.codepoint('A') + nextTarget) }],
      },
    },
}
