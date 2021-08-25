local grafana = import 'grafonnet/grafana.libsonnet';


{
  dashboard:
    local d = grafana.dashboard.new(
      'Pyroscope Server',
      tags=['pyroscope'],
      time_from='now-1h',
      uid='tsWRL6ReZQkirFirmyvnWX1akHXJeHT8I8emjGJo',
      editable='true',
      refresh = if $._config.benchmark then '5s' else '',
    );

    // conditionally add a benchmark rowat the top if appropriate
    local dashboard = if $._config.benchmark then
      d
      .addRow(
      grafana.row.new(
        title='Benchmark',
      )
      .addPanel(
        grafana.text.new(
          content= "<iframe style=\"border:none; width:100%; height: 100%;\" src=\"http://localhost:8081/summary\">",
          span=3,
        )
      )
      .addPanel(
        grafana.gaugePanel.new(
          'Run Progress',
          datasource='$PROMETHEUS_DS',
          unit='percentunit',
          reducerFunction='lastNotNull',
          min=0,
          max=1,
        )
        .addThreshold({ color: 'green', value: 0 })
        .addTarget(
          grafana.prometheus.target(
            'pyroscope_benchmark_progress{}',
            legendFormat='{{ __name__ }}',
          )
        )
      )
      .addPanel(
        grafana.graphPanel.new(
          'Upload Errors (Total)',
          datasource='$PROMETHEUS_DS',
          span=2,
        )
        .addTarget(
          grafana.prometheus.target(
            'pyroscope_benchmark_upload_errors{}',
            legendFormat='{{ __name__ }}',
          )
        )
      )
      .addPanel(
        grafana.graphPanel.new(
          'Successful Uploads (Total)',
          datasource='$PROMETHEUS_DS',
          span=2,
        )
        .addTarget(
          grafana.prometheus.target(
            'pyroscope_benchmark_successful_uploads{}',
            legendFormat='{{ __name__ }}',
          )
        )
      )
    )
    else d;

    dashboard
    .addTemplate(
      grafana.template.datasource(
        name='PROMETHEUS_DS',
        query='prometheus',
        current='prometheus',
        hide='hidden',  // anything other than '' and 'label works
      )
    )
    .addTemplate(
      grafana.template.new(
        'instance',
        '$PROMETHEUS_DS',
        'label_values(pyroscope_build_info, instance)',
        // otherwise the variable may be unpopulated
        // eg. when prometheus/grafana/pyroscope are started at the same time
        refresh='time', 
        label='instance',
      )
    )

    .addRow(
      grafana.row.new(
        title='Meta',
      )
      .addPanel(
        grafana.tablePanel.new(
          title='',
          datasource='$PROMETHEUS_DS',
          span=12,
          height=10,
        )
        // they don't provide any value
        .hideColumn("__name__")
        .hideColumn("Time")
        .hideColumn("instance")
        .hideColumn("Value")
        .hideColumn("job")

        // somewhat useful but preferred to be hidden
        // to make the table cleaner
        .hideColumn("use_embedded_assets")
        .addTarget(
          grafana.prometheus.target(
            'pyroscope_build_info{%s}' % $._config.selector,
            instant=true,
            format='table',
          )
        )
      )
    )


    // Only useful when running benchmark


    .addRow(
      grafana.row.new(
        title='General',
      )
      .addPanel(
        grafana.graphPanel.new(
          'Request Latency P99',
          datasource='$PROMETHEUS_DS',
          format='seconds',
        )
        .addTarget(grafana.prometheus.target(|||
            histogram_quantile(0.99,
              sum(rate(pyroscope_http_request_duration_seconds_bucket{
                instance="$instance",
                handler!="/metrics",
                handler!="/healthz"
              }[$__rate_interval]))
              by (le, handler)
            )
          |||,
          legendFormat='{{ handler }}',
        ))
      )

      .addPanel(
        grafana.graphPanel.new(
          'Error Rate',
          datasource='$PROMETHEUS_DS',
        )
        .addTarget(grafana.prometheus.target(|||
          sum(rate(pyroscope_http_request_duration_seconds_count
          {instance="$instance", code=~"5..", handler!="/metrics", handler!="/healthz"}[$__rate_interval])) by (handler)
          /
          sum(rate(pyroscope_http_request_duration_seconds_count{instance="$instance", handler!="/metrics", handler!="/healthz"}[$__rate_interval])) by (handler)
        |||,
          legendFormat='{{ handler }}',
        ))
      )

      .addPanel(
        grafana.graphPanel.new(
          'Throughput',
          datasource='$PROMETHEUS_DS',
        )
        .addTarget(grafana.prometheus.target('sum(rate(pyroscope_http_request_duration_seconds_count{instance="$instance", handler!="/metrics", handler!="/healthz"}[$__rate_interval])) by (handler)',
          legendFormat='{{ handler }}',
        ))
      )

      .addPanel(
        grafana.graphPanel.new(
          'Response Size P99',
          datasource='$PROMETHEUS_DS',
          format='bytes',
        )
        .addTarget(grafana.prometheus.target('histogram_quantile(0.95, sum(rate(pyroscope_http_response_size_bytes_bucket{instance="$instance", handler!="/metrics", handler!="/healthz"}[$__rate_interval])) by (le, handler))',
          legendFormat='{{ handler }}',
        ))
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
            'pyroscope_cpu_utilization{instance="$instance"}',
          )
        )
      )

    )


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

    // inspired by
    // https://github.com/aukhatov/grafana-dashboards/blob/master/Go%20Metrics-1567509764849.json
    .addRow(
      grafana.row.new(
        title='Go',
        collapse=if $._config.benchmark then false else true,
      )
      .addPanel(
        grafana.graphPanel.new(
          'Memory Off-heap',
          datasource='$PROMETHEUS_DS',
          format='bytes',
        )
        .addTarget(
          grafana.prometheus.target(
            'go_memstats_mspan_inuse_bytes{instance="$instance"}',
            legendFormat='{{ __name__ }}',
          )
        )
        .addTarget(
          grafana.prometheus.target(
            'go_memstats_mspan_sys_bytes{instance="$instance"}',
            legendFormat='{{ __name__ }}',
          )
        )
        .addTarget(grafana.prometheus.target(
          'go_memstats_mcache_inuse_bytes{instance="$instance"}',
          legendFormat='{{ __name__ }}',
        ))
        .addTarget(grafana.prometheus.target(
          'go_memstats_mcache_sys_bytes{instance="$instance"}',
          legendFormat='{{ __name__ }}',
        ))
        .addTarget(grafana.prometheus.target(
          'go_memstats_buck_hash_sys_bytes{instance="$instance"}',
          legendFormat='{{ __name__ }}',
        ))
        .addTarget(grafana.prometheus.target(
          'go_memstats_gc_sys_bytes{instance="$instance"}',
          legendFormat='{{ __name__ }}',
        ))
        .addTarget(grafana.prometheus.target(
          'go_memstats_other_sys_bytes{instance="$instance"}',
          legendFormat='{{ __name__ }}',
        ))
        .addTarget(grafana.prometheus.target(
          'go_memstats_next_gc_bytes{instance="$instance"}',
          legendFormat='{{ __name__ }}',
        ))
      )

      .addPanel(
        grafana.graphPanel.new(
          'Memory In Heap',
          datasource='$PROMETHEUS_DS',
          format='bytes',
        )
        .addTarget(grafana.prometheus.target(
          'go_memstats_heap_alloc_bytes{instance="$instance"}',
          legendFormat='{{ __name__ }}',
        ))
        .addTarget(grafana.prometheus.target(
          'go_memstats_heap_sys_bytes{instance="$instance"}',
          legendFormat='{{ __name__ }}',
        ))
        .addTarget(grafana.prometheus.target(
          'go_memstats_heap_idle_bytes{instance="$instance"}',
          legendFormat='{{ __name__ }}',
        ))
        .addTarget(grafana.prometheus.target(
          'go_memstats_heap_inuse_bytes{instance="$instance"}',
          legendFormat='{{ __name__ }}',
        ))
        .addTarget(grafana.prometheus.target(
          'go_memstats_heap_released_bytes{instance="$instance"}',
          legendFormat='{{ __name__ }}',
        ))
      )


      .addPanel(
        grafana.graphPanel.new(
          'Memory In Stack',
          datasource='$PROMETHEUS_DS',
          format='decbytes',
        )
        .addTarget(
          grafana.prometheus.target(
            'go_memstats_stack_inuse_bytes{instance="$instance"}',
            legendFormat='{{ __name__ }}',
          )
        )
        .addTarget(
          grafana.prometheus.target(
            'go_memstats_stack_sys_bytes{instance="$instance"}',
            legendFormat='{{ __name__ }}',
          )
        )
      )



      .addPanel(
        grafana.graphPanel.new(
          'Total Used Memory',
          datasource='$PROMETHEUS_DS',
          format='decbytes',
        )
        .addTarget(grafana.prometheus.target(
          'go_memstats_sys_bytes{instance="$instance"}',
          legendFormat='{{ __name__ }}',
        ))
      )


      .addPanel(
        grafana.graphPanel.new(
          'Number of Live Objects',
          datasource='$PROMETHEUS_DS',
          legend_show=false,
        )
        .addTarget(grafana.prometheus.target(
          'go_memstats_mallocs_total{instance="$instance"} - go_memstats_frees_total{instance="$instance"}'
        ))
      )

      .addPanel(
        grafana.graphPanel.new(
          'Rate of Objects Allocated',
          datasource='$PROMETHEUS_DS',
          legend_show=false,
        )
        .addTarget(grafana.prometheus.target('rate(go_memstats_mallocs_total{instance="$instance"}[$__rate_interval])'))
      )

      .addPanel(
        grafana.graphPanel.new(
          'Rates of Allocation',
          datasource='$PROMETHEUS_DS',
          format="Bps",
          legend_show=false,
        )
        .addTarget(grafana.prometheus.target('rate(go_memstats_alloc_bytes_total{instance="$instance"}[$__rate_interval])'))
      )

      .addPanel(
        grafana.graphPanel.new(
          'Goroutines',
          datasource='$PROMETHEUS_DS',
          legend_show=false,
        )
        .addTarget(grafana.prometheus.target('go_goroutines{instance="$instance"}'))
      )

      .addPanel(
        grafana.graphPanel.new(
          'GC duration quantile',
          datasource='$PROMETHEUS_DS',
          legend_show=false,
        )
        .addTarget(grafana.prometheus.target('go_gc_duration_seconds{instance="$instance"}'))
      )

      .addPanel(
        grafana.graphPanel.new(
          'File descriptors',
          datasource='$PROMETHEUS_DS',
        )
        .addTarget(grafana.prometheus.target(
          'process_open_fds{instance="$instance"}',
          legendFormat='{{ __name__ }}',
        ))
        .addTarget(grafana.prometheus.target(
          'process_max_fds{instance="$instance"}',
          legendFormat='{{ __name__ }}',
        ))
      )
    )
}
