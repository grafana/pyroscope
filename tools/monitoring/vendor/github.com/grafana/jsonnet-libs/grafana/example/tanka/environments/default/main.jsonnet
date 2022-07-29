local grafana = import '../../../grafana.libsonnet';
local dashboards = import 'dashboards.libsonnet';
local datasources = import 'datasources.libsonnet';
local mixins = import 'mixins.libsonnet';

grafana
+ grafana.withReplicas(3)
+ grafana.withImage('grafana/grafana:8.0.0')
+ grafana.withRootUrl('http://grafana.example.com')
+ grafana.withTheme('dark')
+ grafana.withAnonymous()

// Plugins
+ grafana.addPlugin('fetzerch-sunandmoon-datasource')

// Datasources
+ grafana.addDatasource('Prometheus', datasources.prometheus)
+ grafana.addDatasource('NYC', datasources.sun_and_moon)

// Dashboards
+ grafana.addDashboard('node-exporter-full', dashboards.node_exporter, 'Node Exporter')
+ grafana.addDashboard('nyc', dashboards.nyc, 'Sun and Moon')

// Mixins
+ grafana.addMixinDashboards(mixins)
