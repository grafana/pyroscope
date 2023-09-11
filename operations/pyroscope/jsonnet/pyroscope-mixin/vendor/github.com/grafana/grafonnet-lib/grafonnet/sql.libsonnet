{
  /**
   * Creates an SQL target.
   *
   * @name sql.target
   *
   * @param rawSql The SQL query
   * @param datasource (optional)
   * @param format (default `'time_series'`)
   * @param alias (optional)
   */
  target(
    rawSql,
    datasource=null,
    format='time_series',
    alias=null,
  ):: {
    [if datasource != null then 'datasource']: datasource,
    format: format,
    [if alias != null then 'alias']: alias,
    rawSql: rawSql,
  },
}
