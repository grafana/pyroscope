# node-exporter jsonnet library

Jsonnet library for [node-exporter](https://github.com/kubernetes/node-exporter).

## Usage

Install it with jsonnet-bundler:

```console
jb install github.com/grafana/jsonnet-libs/node-exporter
```

Import into your jsonnet:

```jsonnet
// environments/default/main.jsonnet
local node_exporter = import 'github.com/grafana/jsonnet-libs/node-exporter/main.libsonnet';

{
  node_exporter:
    node_exporter.new()
    + node_exporter.mountRoot(),

  prometheus_config+: {
    scape_configs+: [
      node_exporter.scrape_config(namespace),
    ],
  },
}
```
