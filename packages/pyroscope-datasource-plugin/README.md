# Pyroscope Grafana Panel Plugin

**Important: Grafana version 7.2 or later required**

## Getting started

1. Install the plugin (Installation tab)
2. Install [panel plugin](https://grafana.com/grafana/plugins/pyroscope-panel/)
3. Open Grafana ang go to **Configuratin -> Plugins**
4. Check that plugins are available
5. Set up data source plugin:
   - **Configuration -> Data Sources -> Add data source**
   - click on `pyroscope-datasource`
   - Specify Pyroscope host in `Endpoint` field:
     ![endpoint](https://raw.githubusercontent.com/pyroscope-io/grafana-panel-plugin/main/docs/assets/endpoint.jpg)
6. Set up panel plugin:
   - Add an empty panel on your dashboard
   - Select `pyroscope-panel` from Visualization list
   - Under panel view in Query tab select `pyroscope-datasource`
   - In `Application name` input specify app name
   - Click `Apply`
     ![settings](https://raw.githubusercontent.com/pyroscope-io/grafana-panel-plugin/main/docs/assets/settings.jpg)

Congratulations! Now you can monitor application flamegraph on your Grafana dashboard!
![dashboard](https://raw.githubusercontent.com/pyroscope-io/grafana-panel-plugin/main/docs/assets/dashboard.jpg)

### Datasource template variables

You can use variables of the type **Query** to query Pyroscope for a list of applications, labels, or label values.
Variable query functions supported by Pyroscope data source:

| Name                                  | Description                                              |
|---------------------------------------|----------------------------------------------------------|
| apps()                                | Returns a list of application names                      |
| label_names(*app_name*)               | Returns a list of label names for the application        |
| label_values(*app_name*,*label_name*) | Returns a list of label values for the application label |

#### Example

Create a new variable in the dashboard settings:
 - Name: `appName`
 - Type: **Query**
 - Data source: **select your Pyroscope data source**
 - Query: `apps()`

Now you should see the list of applications returned from the Pyroscope data source, and use the variable in the panel **Query** field:

```
$appName{}
```

