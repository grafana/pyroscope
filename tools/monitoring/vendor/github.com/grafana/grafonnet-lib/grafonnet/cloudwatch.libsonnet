{
  /**
   * Creates a [CloudWatch target](https://grafana.com/docs/grafana/latest/datasources/cloudwatch/)
   *
   * @name cloudwatch.target
   *
   * @param region
   * @param namespace
   * @param metric
   * @param datasource (optional)
   * @param statistic (default: `'Average'`)
   * @param alias (optional)
   * @param highResolution (default: `false`)
   * @param period (default: `'auto'`)
   * @param dimensions (optional)
   * @param id (optional)
   * @param expression (optional)
   * @param hide (optional)

   * @return Panel target
   */

  target(
    region,
    namespace,
    metric,
    datasource=null,
    statistic='Average',
    alias=null,
    highResolution=false,
    period='auto',
    dimensions={},
    id=null,
    expression=null,
    hide=null
  ):: {
    region: region,
    namespace: namespace,
    metricName: metric,
    [if datasource != null then 'datasource']: datasource,
    statistics: [statistic],
    [if alias != null then 'alias']: alias,
    highResolution: highResolution,
    period: period,
    dimensions: dimensions,
    [if id != null then 'id']: id,
    [if expression != null then 'expression']: expression,
    [if hide != null then 'hide']: hide,

  },
}
