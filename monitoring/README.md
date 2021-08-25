# Monitoring

1. Install jsonnet/jb by running `make install-dev-tools` in the root Makefile.

2. Install dependencies
```
make init
```

3. (Re)Generate the dashboard
```
make dashboard
```

# Development


Run the [grafana-integration](../examples/grafana-integration) example docker-compose then copy the generated dashboard there:

```
make dashboard && \
  cp dashboard.json ../examples/grafana-integration/grafana-provisioning/dashboards/ && \
  docker-compose -f ../examples/grafana-integration/docker-compose.yml up -d --force-recreate grafana
```

# References

* https://grafana.github.io/grafonnet-lib/api-docs/
* https://github.com/grafana/grafonnet-lib/tree/master/grafonnet
* https://github.com/kubernetes-monitoring/kubernetes-mixin
