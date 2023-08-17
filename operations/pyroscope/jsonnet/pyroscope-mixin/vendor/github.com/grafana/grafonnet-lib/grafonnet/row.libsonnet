{
  /**
   * Creates a [row](https://grafana.com/docs/grafana/latest/features/dashboard/dashboards/#rows).
   * Rows are logical dividers within a dashboard and used to group panels together.
   *
   * @name row.new
   *
   * @param title The title of the row.
   * @param showTitle (default `true` if title is set) Whether to show the row title
   * @paral titleSize (default `'h6'`) The size of the title
   * @param collapse (default `false`) The initial state of the row when opening the dashboard. Panels in a collapsed row are not load until the row is expanded.
   * @param repeat (optional) Name of variable that should be used to repeat this row. It is recommended to use the variable in the row title as well.
   *
   * @method addPanels(panels) Appends an array of nested panels
   * @method addPanel(panel,gridPos) Appends a nested panel, with an optional grid position in grid coordinates, e.g. `gridPos={'x':0, 'y':0, 'w':12, 'h': 9}`
   */
  new(
    title='Dashboard Row',
    height=null,
    collapse=false,
    repeat=null,
    showTitle=null,
    titleSize='h6'
  ):: {
    collapse: collapse,
    collapsed: collapse,
    [if height != null then 'height']: height,
    panels: [],
    repeat: repeat,
    repeatIteration: null,
    repeatRowId: null,
    showTitle:
      if showTitle != null then
        showTitle
      else
        title != 'Dashboard Row',
    title: title,
    type: 'row',
    titleSize: titleSize,
    addPanels(panels):: self {
      panels+: panels,
    },
    addPanel(panel, gridPos={}):: self {
      panels+: [panel { gridPos: gridPos }],
    },
  },
}
