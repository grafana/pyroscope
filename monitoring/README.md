# Monitoring

1. Install jsonnet/jb by running `make install-dev-tools` in the root Makefile.

2. Install dependencies
```
make init
```

3. (Re)Generate the dashboards
```
make
```

# Development


Run the [grafana-integration](../examples/grafana-integration) example docker-compose then copy the generated dashboard there:

```
make && docker-compose -f ../examples/grafana-integration/docker-compose.yml up -d --force-recreate grafana
```

# Warnings
* If you ever rename the dashboard path, don't forget to update the references (see all the docker-compose.yaml)

# References

* https://grafana.github.io/grafonnet-lib/api-docs/
* https://github.com/grafana/grafonnet-lib/tree/master/grafonnet
* https://github.com/kubernetes-monitoring/kubernetes-mixin
