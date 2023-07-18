# Prometheus Ksonnet

A set of extensible configs for running Prometheus on Kubernetes.

Usage:
- Make sure you have [Tanka](https://tanka.dev/install) and
  [jsonnet-bundler](https://tanka.dev/install#jsonnet-bundler) installed:

- In your config repo, init Tanka with [`k8s-libsonnet`](https://github.com/jsonnet-libs/k8s-libsonnet) and point it at your Kubernetes cluster:

```bash
# set up Tanka project
tk init --k8s=1.18

# point at cluster
export CONTEXT=$(kubectl config current-context)
tk env set environments/default  --server-from-context=$CONTEXT
```

- Vendor this package using jsonnet-bundler:

```bash
jb install github.com/grafana/jsonnet-libs/prometheus-ksonnet
```

- Assuming you want to run in the default namespace ('environment' in Tanka parlance), add the following to the file `environments/default/main.jsonnet`:

```bash
cat <<EOF > environments/default/main.jsonnet
local prometheus = import "prometheus-ksonnet/prometheus-ksonnet.libsonnet";

prometheus {
  _config+:: {
    cluster_name: "cluster1",
    namespace: "default",
  },
}
EOF
```

- Apply your config:

```bash
tk apply environments/default
```

# Kops

If you made your Kubernetes cluster with [Kops](https://github.com/kubernetes/kops),
add the Kops mixin to your config to scrape the Kubernetes system components:

```jsonnet
local prometheus = import "prometheus-ksonnet/prometheus-ksonnet.libsonnet";
local kops = import "prometheus-ksonnet/lib/prometheus-config-kops.libsonnet";

prometheus + kops {
  _config+:: {
    namespace: "default",
    insecureSkipVerify: true,
  },
}
```

# Customising and Extending.

The choice of Tanka for configuring these jobs was intentional; it allows users
to easily override setting in these configurations to suit their needs, without having
to fork or modify this library.  For instance, to override the resource requests
and limits for the Prometheus container, you would:

```jsonnet
local prometheus = import "prometheus-ksonnet/prometheus-ksonnet.libsonnet";

prometheus {
  prometheus_container+::
     $.util.resourcesRequests("1", "2Gi") +
     $.util.resourcesLimits("2", "4Gi"),
}
```

We sometimes specify config options in a `_config` dict; there are two situations
under which we do this:

- When you must provide a value for the parameter (such as `namesapce`).
- When the parameter get referenced in multiple places, and overriding it using
  the technique above would be cumbersome and error prone (such as with `cluster_dns_suffix`).

We use these two guidelines for when to put parameters in `_config` as otherwise
the config field would just become the same as the jobs it declares - and lets
not forget, this whole thing is config - so its completely acceptable to override
pretty much any of it.
