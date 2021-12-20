# Pyroscope Grafana Panel Plugin

**Important: Grafana version 7.2 or later required**

## Getting started

1. Install the plugin (Installation tab)
2. Install [datasource plugin](https://grafana.com/grafana/plugins/pyroscope-datasource/)
3. Open Grafana and go to **Configuration -> Plugins**
4. Check that plugins are available
5. Set up data source plugin:
   * **Configuration -> Data Sources -> Add data source**
   * click on `pyroscope-datasource`
   * Specify Pyroscope host in `Endpoint` field
6. Set up panel plugin:
    * Add an empty panel on your dashboard
    * Select `pyroscope-panel` from Visualization list
    * Under panel view in Query tab select `pyroscope-datasource`
    * In `Application name` input specify app name
    * Click `Apply`

Congratulations! Now you can monitor application flamegraph on your Grafana dashboard!
