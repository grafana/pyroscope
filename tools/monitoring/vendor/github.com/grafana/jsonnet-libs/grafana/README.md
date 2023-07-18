# Jsonnet library for deploying Grafana

This library aims to simplify the deployment of Grafana into Kubernetes
via Jsonnet.

As well as deploying Grafana itself, it also supports the deployment of
dashboards, datasources, notification channels and plugins, all from
within your Jsonnet code.

## Library Notes
### Statefulness
Currently, this library creates a stateless Grafana. What this actually
means is that, on startup, Grafana imports dashboard files into its
database. However, given Grafana is installed into a deployment without
an external volume, the database is wiped on restart. This creates a
stateless Grafana installation.

Statelessness is a **good thing** when deploying all your dashboards,
datasources, plugins and notification channels from Jsonnet code.
Through it you avoid your dashboards drifting from the Jsonnet/version
control managed known good state.

However, a possible future extension of this library could convert the
Grafana instance into a stateful one, mounting the database into a PVC.

### Mixins
This library does not (yet) support [Monitoring Mixins](https://github.com/monitoring-mixins/docs) directly, although
it has much of the mechanics required to do so.

## An Example
```
local grafana = import '../grafana.libsonnet';
local k = import 'k.libsonnet';
{
  config+:: {
    prometheus_url: 'http://prometheus',
  },

  namespace: k.core.v1.namespace.new('grafana'),

  prometheus_datasource:: grafana.datasource.new('prometheus', $.config.prometheus_url, type='prometheus', default=true),

  grafana: grafana
           + grafana.withAnonymous()
           + grafana.addFolder('Example')
           + grafana.addDashboard('simple', (import 'dashboard-simple.libsonnet'), folder='Example')
           + grafana.addDatasource('prometheus', $.prometheus_datasource),
}
```
