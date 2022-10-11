{
  /**
   * Creates a [Prometheus target](https://grafana.com/docs/grafana/latest/datasources/prometheus/)
   * to be added to panels.
   *
   * @name prometheus.target
   *
   * @param expr PromQL query to be exercised against Prometheus. Checkout [Prometheus documentation](https://prometheus.io/docs/prometheus/latest/querying/basics/).
   * @param format (default `'time_series'`) Switch between `'table'`, `'time_series'` or `'heatmap'`. Table will only work in the Table panel. Heatmap is suitable for displaying metrics of the Histogram type on a Heatmap panel. Under the hood, it converts cumulative histograms to regular ones and sorts series by the bucket bound.
   * @param intervalFactor (default `2`)
   * @param legendFormat (default `''`) Controls the name of the time series, using name or pattern. For example `{{hostname}}` is replaced with the label value for the label `hostname`.
   * @param datasource (optional) Name of the Prometheus datasource. Leave by default otherwise.
   * @param interval (optional) Time span used to aggregate or group data points by time. By default Grafana uses an automatic interval calculated based on the width of the graph.
   * @param instant (optional) Perform an "instant" query, to return only the latest value that Prometheus has scraped for the requested time series. Instant queries return results much faster than normal range queries. Use them to look up label sets.
   * @param hide (optional) Set to `true` to hide the target from the panel.
   *
   * @return A Prometheus target to be added to panels.
   */
  target(
    expr,
    format='time_series',
    intervalFactor=2,
    legendFormat='',
    datasource=null,
    interval=null,
    instant=null,
    hide=null,
  ):: {
    [if hide != null then 'hide']: hide,
    [if datasource != null then 'datasource']: datasource,
    expr: expr,
    format: format,
    intervalFactor: intervalFactor,
    legendFormat: legendFormat,
    [if interval != null then 'interval']: interval,
    [if instant != null then 'instant']: instant,
  },
}
