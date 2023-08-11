{
  /**
   * Creates a Timepicker
   *
   * @name timepicker.new
   *
   * @param refresh_intervals (default: `['5s','10s','30s','1m','5m','15m','30m','1h','2h','1d']`) Array of time durations
   * @param time_options (default: `['5m','15m','1h','6h','12h','24h','2d','7d','30d']`) Array of time durations
   */
  new(
    refresh_intervals=[
      '5s',
      '10s',
      '30s',
      '1m',
      '5m',
      '15m',
      '30m',
      '1h',
      '2h',
      '1d',
    ],
    time_options=[
      '5m',
      '15m',
      '1h',
      '6h',
      '12h',
      '24h',
      '2d',
      '7d',
      '30d',
    ],
    nowDelay=null,
  ):: {
    refresh_intervals: refresh_intervals,
    time_options: time_options,
    [if nowDelay != null then 'nowDelay']: nowDelay,
  },
}
