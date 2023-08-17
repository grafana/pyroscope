{
  /**
   * Creates an [Alert list panel](https://grafana.com/docs/grafana/latest/panels/visualizations/alert-list-panel/)
   *
   * @name alertlist.new
   *
   * @param title (default `''`)
   * @param span (optional)
   * @param show (default `'current'`) Whether the panel should display the current alert state or recent alert state changes.
   * @param limit (default `10`) Sets the maximum number of alerts to list.
   * @param sortOrder (default `'1'`) '1': alerting, '2': no_data, '3': pending, '4': ok, '5': paused
   * @param stateFilter (optional)
   * @param onlyAlertsOnDashboard (optional) Shows alerts only from the dashboard the alert list is in
   * @param transparent (optional) Whether to display the panel without a background
   * @param description (optional)
   * @param datasource (optional)
   */
  new(
    title='',
    span=null,
    show='current',
    limit=10,
    sortOrder=1,
    stateFilter=[],
    onlyAlertsOnDashboard=true,
    transparent=null,
    description=null,
    datasource=null,
  )::
    {
      [if transparent != null then 'transparent']: transparent,
      title: title,
      [if span != null then 'span']: span,
      type: 'alertlist',
      show: show,
      limit: limit,
      sortOrder: sortOrder,
      [if show != 'changes' then 'stateFilter']: stateFilter,
      onlyAlertsOnDashboard: onlyAlertsOnDashboard,
      [if description != null then 'description']: description,
      datasource: datasource,
    },
}
