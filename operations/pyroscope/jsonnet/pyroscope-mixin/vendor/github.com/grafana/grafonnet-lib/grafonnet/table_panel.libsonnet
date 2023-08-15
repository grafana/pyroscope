{
  /**
   * Creates a [table panel](https://grafana.com/docs/grafana/latest/panels/visualizations/table-panel/) that can be added in a row.
   * It requires the table panel plugin in grafana, which is built-in.
   *
   * @name table.new
   *
   * @param title The title of the graph panel.
   * @param description (optional) Description of the panel
   * @param span (optional)  Width of the panel
   * @param height (optional)  Height of the panel
   * @param datasource (optional) Datasource
   * @param min_span (optional)  Min span
   * @param styles (optional) Array of styles for the panel
   * @param columns (optional) Array of columns for the panel
   * @param sort (optional) Sorting instruction for the panel
   * @param transform (optional) Allow table manipulation to present data as desired
   * @param transparent (default: 'false') Whether to display the panel without a background
   * @param links (optional) Array of links for the panel.
   * @return A json that represents a table panel
   *
   * @method addTarget(target) Adds a target object
   * @method addTargets(targets) Adds an array of targets
   * @method addColumn(field, style) Adds a column
   * @method hideColumn(field) Hides a column
   * @method addLink(link) Adds a link
   * @method addTransformation(transformation) Adds a transformation object
   * @method addTransformations(transformations) Adds an array of transformations
   */
  new(
    title,
    description=null,
    span=null,
    min_span=null,
    height=null,
    datasource=null,
    styles=[],
    transform=null,
    transparent=false,
    columns=[],
    sort=null,
    time_from=null,
    time_shift=null,
    links=[],
  ):: {
    type: 'table',
    title: title,
    [if span != null then 'span']: span,
    [if min_span != null then 'minSpan']: min_span,
    [if height != null then 'height']: height,
    datasource: datasource,
    targets: [
    ],
    styles: styles,
    columns: columns,
    timeFrom: time_from,
    timeShift: time_shift,
    links: links,
    [if sort != null then 'sort']: sort,
    [if description != null then 'description']: description,
    [if transform != null then 'transform']: transform,
    [if transparent == true then 'transparent']: transparent,
    _nextTarget:: 0,
    addTarget(target):: self {
      local nextTarget = super._nextTarget,
      _nextTarget: nextTarget + 1,
      targets+: [target { refId: std.char(std.codepoint('A') + nextTarget) }],
    },
    addTargets(targets):: std.foldl(function(p, t) p.addTarget(t), targets, self),
    addColumn(field, style):: self {
      local style_ = style { pattern: field },
      local column_ = { text: field, value: field },
      styles+: [style_],
      columns+: [column_],
    },
    hideColumn(field):: self {
      styles+: [{
        alias: field,
        pattern: field,
        type: 'hidden',
      }],
    },
    addLink(link):: self {
      links+: [link],
    },
    addTransformation(transformation):: self {
      transformations+: [transformation],
    },
    addTransformations(transformations):: std.foldl(function(p, t) p.addTransformation(t), transformations, self),
  },
}
