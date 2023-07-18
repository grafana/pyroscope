# kube-state-metrics jsonnet library

Jsonnet library for [kube-state-metrics](https://github.com/kubernetes/kube-state-metrics).

## Usage

Install it with jsonnet-bundler:

```console
jb install github.com/grafana/jsonnet-libs/kube-state-metrics
```

Import into your jsonnet:

```jsonnet
// environments/default/main.jsonnet
local ksm = import 'github.com/grafana/jsonnet-libs/kube-state-metrics/main.libsonnet';

{
  local namespace = 'default',
  ksm: ksm.new(namespace),

  prometheus_config+: {
    scrape_configs+: [
      ksm.scrape_config(namespace),
    ],
  },
}
```
