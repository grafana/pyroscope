# Alertmanager Mixin

The Alertmanager Mixin is a set of configurable, reusable, and extensible
alerts (and eventually dashboards) for Alertmanager.

The alerts are designed to monitor a cluster of Alertmanager instances. To make
them work as expected, the Prometheus server the alerts are evaluated on has to
scrape all Alertmanager instances of the cluster, even if those instances are
distributed over different locations. All Alertmanager instances in the same
Alertmanager cluster must have the same `job` label. In turn, if monitoring
multiple different Alertmanager clusters, instances from different clusters
must have a different `job` label.

The most basic use of the Alertmanager Mixin is to create a YAML file with the
alerts from it. To do so, you need to have `jsonnetfmt` and `mixtool` installed. If you have a working Go development environment, it's
easiest to run the following:

```bash
$ go get github.com/monitoring-mixins/mixtool/cmd/mixtool
$ go get github.com/google/go-jsonnet/cmd/jsonnetfmt
```

Edit `config.libsonnet` to match your environment and then build
`alertmanager_alerts.yaml` with the alerts by running:

```bash
$ make build
```

For instructions on more advanced uses of mixins, see https://github.com/monitoring-mixins/docs.
