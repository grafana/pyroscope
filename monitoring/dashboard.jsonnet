local grafana = import 'grafonnet/grafana.libsonnet';

// TODO
// add transformations once this pr is merged
// https://github.com/grafana/grafonnet-lib/pull/324
// match: go_memstats_(.*)_bytes
// replace: $1

grafana.dashboard.new(
  'Pyroscope Server',
  tags=['pyroscope'],
  time_from='now-1h',
  uid='tsWRL6ReZQkirFirmyvnWX1akHXJeHT8I8emjGJo',
  editable='true',  // TODO: remove
)

.addTemplate(
  grafana.template.datasource(
    'PROMETHEUS_DS',
    'prometheus',
    '',
    hide='hidden',  // anything other than '' and 'label works
  )
)
// TODO
// replace cache_trees_size for pyroscope_info or something
.addTemplate(
  grafana.template.new(
    'instance',
    '$PROMETHEUS_DS',
    'label_values(pyroscope_cache_size, instance)',
    label='instance',
  )
)

.addRow(
  grafana.row.new(
    title='CPU',
  )
  .addPanel(
    grafana.graphPanel.new(
      'CPU Utilization',
      datasource='$PROMETHEUS_DS',
      format='percent',
      min='0',
      max='100',
      legend_show=false,
    )
    .addTarget(
      grafana.prometheus.target(
        'pyroscope_cpu_utilization{}',
      )
    )
  )


  .addPanel(
    grafana.graphPanel.new(
      'Goroutines',
      datasource='$PROMETHEUS_DS',
      min='0',
      legend_show=false,
    )
    .addTarget(
      grafana.prometheus.target(
        '{__name__=~"go_goroutines", instance="$instance"}',
      )
    )
  )

  .addPanel(
    grafana.graphPanel.new(
      'Throughput',
      datasource='$PROMETHEUS_DS',
      format='rps',
    )
    .addTarget(
      grafana.prometheus.target(
        'rate(storage_reads_total{}[1m])',
      )
    )
  )
)


.addRow(
  grafana.row.new(
    title='Memory',
  )
  .addPanel(
    grafana.graphPanel.new(
      'Memory',
      datasource='$PROMETHEUS_DS',
      legend_values='true',
      legend_rightSide='true',
      legend_alignAsTable='true',
      legend_current='true',
      legend_sort='current',
      legend_sortDesc=true,
      format='bytes',
    )
    .addTarget(
      grafana.prometheus.target(
        '{__name__=~"go_memstats_.+bytes", instance="$instance"}',
        legendFormat='{{__name__}}',
      ),
    )
    .addTarget(
      grafana.prometheus.target(
        'evictions_total_bytes{}',
        legendFormat='RAM total',
      ),
    )
  )


  .addPanel(
    grafana.graphPanel.new(
      'Cache Size',
      description='Number of objects in cache',
      datasource='$PROMETHEUS_DS',
      legend_values='true',
      legend_rightSide='true',
      legend_alignAsTable='true',
      legend_current='true',
      legend_sort='current',
      legend_sortDesc=true,
      min='0',
    )
    .addTarget(
      grafana.prometheus.target(
        '{__name__=~"cache_.+_size"}',
        legendFormat='{{__name__}}',
      ),
    )
  )

  .addPanel(
    grafana.graphPanel.new(
      'Cache Hit Ratios',
      datasource='$PROMETHEUS_DS',
      legend_values='true',
      legend_rightSide='true',
      legend_alignAsTable='true',
      legend_current='true',
      legend_sort='current',
      legend_sortDesc=true,
      format='percentunit',
    )
    .addTarget(
      grafana.prometheus.target(
        'cache_dicts_hit / (cache_dicts_hit+cache_dicts_miss)',
        legendFormat='cache_dicts_hit_rate',
      ),
    )
    .addTarget(
      grafana.prometheus.target(
        'cache_trees_hit / (cache_trees_hit+cache_trees_miss)',
        legendFormat='cache_trees_hit_rate',
      ),
    )
    .addTarget(
      grafana.prometheus.target(
        'cache_segments_hit / (cache_segments_hit+cache_segments_miss)',
        legendFormat='cache_segments_hit_rate',
      ),
    )
    .addTarget(
      grafana.prometheus.target(
        'cache_dimensions_hit / (cache_dimensions_hit+cache_dimensions_miss)',
        legendFormat='cache_dimensions_hit_rate',
      ),
    )
  )

  .addPanel(
    grafana.text.new(
      span='12',
      content=|||
        <p style="text-align: left; font-weight: bold;">
        Evictions & Write Back
        </p>


        Cache objects have two ways of getting persisted:
        * via evictions when there's memory pressure
        * via write-back mechanism that's continiuosly saving data on disk
      |||,
    )
  )
  .addPanel(
    grafana.graphPanel.new(
      'Cache Evictions Timer',
      datasource='$PROMETHEUS_DS',
      legend_values='true',
      format='ns',
    )
    .addTarget(
      grafana.prometheus.target(
        '{__name__=~"evictions_timer", instance="$instance"}',
        legendFormat='{{__name__}}',
      ),
    )
  )

  .addPanel(
    grafana.graphPanel.new(
      'Cache Write Back Timer',
      datasource='$PROMETHEUS_DS',
      legend_values='true',
      format='ns',
    )
    .addTarget(
      grafana.prometheus.target(
        '{__name__=~"write_back_timer", instance="$instance"}',
        legendFormat='{{__name__}}',
      ),
    )
  )

  .addPanel(
    grafana.graphPanel.new(
      'Cache Write Back Count',
      datasource='$PROMETHEUS_DS',
      legend_values='true',
    )
    .addTarget(
      grafana.prometheus.target(
        '{__name__=~"write_back_count", instance="$instance"}',
        legendFormat='{{__name__}}',
      ),
    )
  )
)

// Storage
.addRow(
  grafana.row.new(
    title='Storage',
  )
  .addPanel(
    grafana.graphPanel.new(
      'Badger Space Breakdown',
      datasource='$PROMETHEUS_DS',
      legend_values='true',
      legend_rightSide='true',
      legend_alignAsTable='true',
      legend_current='true',
      legend_sort='current',
      legend_sortDesc=true,
      format='bytes',
    )
    .addTarget(
      grafana.prometheus.target(
        '{__name__=~"disk_.+"}',
        legendFormat='{{__name__}}',
      ),
    )
  )

  .addPanel(
    grafana.graphPanel.new(
      'Badger Reads',
      datasource='$PROMETHEUS_DS',
      legend_values='true',
      legend_rightSide='true',
      legend_alignAsTable='true',
      legend_current='true',
      legend_sort='current',
      legend_sortDesc=true,
      min='0',
    )
    .addTarget(
      grafana.prometheus.target(
        '{__name__=~"storage_.+_read"}',
        legendFormat='{{__name__}}',
      ),
    )
  )

  .addPanel(
    grafana.graphPanel.new(
      'Badger Writes',
      datasource='$PROMETHEUS_DS',
      legend_values='true',
      legend_rightSide='true',
      legend_alignAsTable='true',
      legend_current='true',
      legend_sort='current',
      legend_sortDesc=true,
      min='0',
    )
    .addTarget(
      grafana.prometheus.target(
        '{__name__=~"storage_.+_write"}',
        legendFormat='{{__name__}}',
      ),
    )
  )
  .addPanel(
    grafana.graphPanel.new(
      'Retention',
      datasource='$PROMETHEUS_DS',
      legend_values='true',
      format='ns',
    )
    .addTarget(
      grafana.prometheus.target(
        'retention_timer',
        legendFormat='{{__name__}}',
      ),
    )
  )
)

// My new stuff
.addRow(
  grafana.row.new(
    title='Storage',
  )
  .addPanel(
    grafana.graphPanel.new(
      'Cache Hit/Misses',
      datasource='$PROMETHEUS_DS',
      legend_values='true',
      legend_rightSide='true',
      legend_alignAsTable='true',
      legend_current='true',
      legend_sort='current',
      legend_sortDesc=true,
      format='percentunit',
    )
    .addTarget(
      grafana.prometheus.target(|||
        sum without(name) (rate(pyroscope_storage_cache_hits_total[$__rate_interval]))
        /
        sum without(name) (rate(pyroscope_storage_cache_reads_total[$__rate_interval]))
      |||,
        legendFormat='Hits',
      )
    )
    .addTarget(
      grafana.prometheus.target(|||
        sum without(name) (rate(pyroscope_storage_cache_misses_total[$__rate_interval]))
        /
        sum without(name)(rate(pyroscope_storage_cache_reads_total[$__rate_interval]))
      |||,
        legendFormat='Misses',
      )
    )
  )
  .addPanel(
    grafana.graphPanel.new(
      'Cache Hit Ratio',
      datasource='$PROMETHEUS_DS',
      legend_values='true',
      legend_rightSide='true',
      legend_alignAsTable='true',
      legend_current='true',
      legend_sort='current',
      legend_sortDesc=true,
      format='percentunit',
    )
    .addTarget(
      grafana.prometheus.target(|||
        rate(pyroscope_storage_cache_hits_total[$__rate_interval])
        /
        rate(pyroscope_storage_cache_reads_total[$__rate_interval])
      |||,
        legendFormat='{{ name }}',
      )
    )
  )
  .addPanel(
    grafana.graphPanel.new(
      'Rate of items persisted from cache to disk',
      datasource='$PROMETHEUS_DS',
      legend_values='true',
      legend_rightSide='true',
      legend_alignAsTable='true',
      legend_current='true',
      legend_sort='current',
      legend_sortDesc=true,
    )
    .addTarget(
      grafana.prometheus.target(
        // ignore the name
        'sum without(name) (rate(pyroscope_storage_cache_persisted_total[$__rate_interval]))',
        legendFormat="Total",
      )
    )
    .addTarget(
      grafana.prometheus.target(
        '(rate(pyroscope_storage_cache_persisted_total[$__rate_interval]))',
        legendFormat="{{ name }}",
      )
    )
  )

  .addPanel(
    grafana.graphPanel.new(
      'Storage Reads/Writes',
      datasource='$PROMETHEUS_DS',
    )
    .addTarget(
      grafana.prometheus.target(
        'rate(pyroscope_storage_reads_total[$__rate_interval])',
        legendFormat="Reads",
      )
    )
    .addTarget(
      grafana.prometheus.target(
        'rate(pyroscope_storage_writes_total[$__rate_interval])',
        legendFormat="Writes",
      )
    )
  )
  .addPanel(
    grafana.graphPanel.new(
      'Periodic tasks',
      datasource='$PROMETHEUS_DS',
      legend_values='true',
      format='seconds',
      min='0.001', // as as the bucket minimum
      max='10', // same as the maximum bucket
      logBase1Y=2,
    )
    .addTarget(
      grafana.prometheus.target(
        'histogram_quantile(0.9, pyroscope_storage_evictions_duration_seconds_bucket)',
        legendFormat='evictions',
      ),
    )
    .addTarget(
      grafana.prometheus.target(
        'histogram_quantile(0.9, pyroscope_storage_writeback_duration_seconds_bucket)',
        legendFormat='write-back',
      ),
    )
    .addTarget(
      grafana.prometheus.target(
        'histogram_quantile(0.9, pyroscope_storage_retention_duration_seconds_bucket)',
        legendFormat='retention',
      ),
    )
  )

  .addPanel(
    grafana.graphPanel.new(
      'Disk Usage',
      datasource='$PROMETHEUS_DS',
      format='bytes',
      legend_values='true',
      legend_rightSide='true',
      legend_alignAsTable='true',
      legend_current='true',
      legend_sort='current',
      legend_sortDesc=true,
    )
    .addTarget(
      grafana.prometheus.target(
        'pyroscope_storage_disk_bytes',
        legendFormat='{{ name }}',
      ),
    )
    .addTarget(
      grafana.prometheus.target(
        'sum without(name)(pyroscope_disk_bytes)',
        legendFormat='total',
      ),
    )
  )

  .addPanel(
    grafana.graphPanel.new(
      'Cache Size (number of items)',
      datasource='$PROMETHEUS_DS',
      legend_values='true',
      legend_rightSide='true',
      legend_alignAsTable='true',
      legend_current='true',
      legend_sort='current',
      legend_sortDesc=true,
    )
    .addTarget(
      grafana.prometheus.target(
        'pyroscope_storage_cache_size',
        legendFormat='{{ name }}',
      ),
    )
    .addTarget(
      grafana.prometheus.target(
        'sum without(name)(pyroscope_storage_cache_size)',
        legendFormat='total',
      ),
    )
  )

  .addPanel(
    grafana.graphPanel.new(
      'Cache Size (Approximation)',
      datasource='$PROMETHEUS_DS',
      format='bytes',
      legend_values='true',
      legend_rightSide='true',
      legend_alignAsTable='true',
      legend_current=true,
      legend_max=true,
      legend_sort='current',
      legend_sortDesc=true,
      logBase1Y=2,
    )
    .addTarget(
      grafana.prometheus.target(
        'pyroscope_storage_evictions_alloc_bytes',
        legendFormat='heap size',
      ),
    )
    .addTarget(
      grafana.prometheus.target(
        'pyroscope_storage_evictions_total_mem_bytes',
        legendFormat='total memory',
      ),
    )
  )
)
