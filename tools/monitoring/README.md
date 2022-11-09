# Monitoring

This folder contains a deployment example of the phlare-mixin.
This is useful for testing new changes to the mixin.

First use `make deploy-micro-services` to deploy locally using kind.

Then use `make deploy-monitoring` to deploy the mixin in the monitoring namespace.

By default it install:

- Prometheus and Alertmanager with alerts and rules.
- Grafana with plugin, datasources and dashboards.
- An nginx proxy to access all applications.

To access the proxy you just need to port forward using `kubectl port-forward deployments.apps/nginx -n monitoring 3100:80`.

> You can replace 3100 with the local port of your choice
