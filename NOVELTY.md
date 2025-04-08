# Period A: Fairly stable consumption
https://ops.grafana-ops.net/a/grafana-pyroscope-app/explore?searchText=&panelType=time-series&layout=grid&hideNoData=off&explorationType=flame-graph&var-serviceName=profiles-prod-001%2Fingester&var-profileMetricId=process_cpu:cpu:nanoseconds:cpu:nanoseconds&var-spanSelector=undefined&var-dataSource=grafanacloud-profiles&var-filters=pod%7C%3D%7Cpyroscope-ingester-0&var-filtersBaseline=&var-filtersComparison=&var-groupBy=&maxNodes=16384&from=2025-04-08T13:20:56.014Z&to=2025-04-08T14:04:10.198Z

$ go run ./cmd/profilecli  query novelty --from 2025-04-08T13:20:56Z --to 2025-04-08T14:04:00Z --query '{namespace="profiles-prod-001", pod="pyroscope-ingester-0"}'


# Period B: Starts to flush

https://ops.grafana-ops.net/a/grafana-pyroscope-app/explore?searchText=&panelType=time-series&layout=grid&hideNoData=off&explorationType=flame-graph&var-serviceName=profiles-prod-001%2Fingester&var-profileMetricId=process_cpu:cpu:nanoseconds:cpu:nanoseconds&var-spanSelector=undefined&var-dataSource=grafanacloud-profiles&var-filters=pod%7C%3D%7Cpyroscope-ingester-0&var-filtersBaseline=&var-filtersComparison=&var-groupBy=&maxNodes=16384&from=2025-04-08T14:04:35.126Z&to=2025-04-08T14:10:54.684Z

$ go run ./cmd/profilecli  query novelty --from 2025-04-08T14:04:34Z --to 2025-04-08T14:10.54:00Z --query '{namespace="profiles-prod-001", pod="pyroscope-ingester-0"}'

