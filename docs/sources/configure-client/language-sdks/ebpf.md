---
title: "eBPF"
menuTitle: "eBPF"
description: "Instrumenting eBPF applications for continuous profiling"
weight: 30
---

# eBPF

## How to profiling your application using eBPF

eBPF is an emerging Linux kernel technology that allows for user-supplied programs to run inside of the kernel. This enables a bunch of interesting use cases, particularly efficient CPU profiling of the whole Linux system.


## Benefits and Tradeoffs of using eBPF for continuous profiling

We added a blog post "[The pros and cons of eBPF profiling](https://pyroscope.io/blog/ebpf-profiling-pros-cons)" which more deeply
explores this topic and provides some examples of eBPF profiles. If you're interested in some of the more granular details you can find them there!

## Getting Started with eBPF Profiling

For the reasons mentioned above, we recommend an hybrid approach for better results: eBPF to profile the node and specific language instrumentation per application. The following steps will explain how to get started:

### Prerequisites for profiling with eBPF
For the eBPF integration to work you'll need:
* A Pyroscope or Phlare server where the agent will send profiling data
* A Linux machine with the kernel version >= 4.9 (due to [BPF_PROG_TYPE_PERF_EVENT](https://lkml.org/lkml/2016/9/1/831))

## Running eBPF profiler on Kubernetes
### Step 1: Add the helm repo

```shell
helm repo add pyroscope-io https://pyroscope-io.github.io/helm-chart
```

### Step 2: Install pyroscope agent

```shell
helm install pyroscope-ebpf pyroscope-io/pyroscope-ebpf
```

It will install pyroscope eBPF agent on all of your nodes and start profiling applications across your cluster.

## Running eBPF profiler from binary
```shell
export PYROSCOPE_APPLICATION_NAME=my.ebpf.program
export PYROSCOPE_SERVER_ADDRESS=http://address-of-pyroscope-server:4040/
export PYROSCOPE_SPY_NAME=ebpfspy
# optionally, if authentication is enabled, specify the API key:
# export PYROSCOPE_AUTH_TOKEN={YOUR_API_KEY}

# to wrap an existing program and profile it
sudo -E pyroscope exec mongod

# to profile the whole system
sudo -E pyroscope ebpf
```

## Dealing with `[unknowns]`

eBPF relies on having debugging symbols available for each program installed in your system. If you don't have those you'll see a lot of stacktraces full of `[unknown]`s. On most systems you can get debugging symbols for most packages with `debuginfo-install` command:

```shell
sudo debuginfo-install -y <pkg>
```

## Configuration

All parameters below are also supported as CLI arguments, a full list can be accessed via `pyroscope ebpf --help`. For brevity only environment variables are listed.

* `PYROSCOPE_KUBERNETES_NODE` Set to current k8s Node.nodeName for service discovery and labeling
* `PYROSCOPE_ONLY_SERVICES` Ignore processes unknown to service discovery
* `PYROSCOPE_SYMBOL_CACHE_SIZE` Max size of symbols cache (1 entry per process)

| env var                    | default                          | description                                    |
| -------------------------- | -------------------------------- | ---------------------------------------------- |
| `PYROSCOPE_KUBERNETES_NODE` | `""` | Used by service discovery. It's automatically set in the Helm Chart. |
| `PYROSCOPE_ONLY_SERVICES`     | `false`                             | Ignore processes unknown to service discovery. In a Kubernetes cluster it ignores processes like `containerd, runc, kubelet` etc | 
| `PYROSCOPE_SYMBOL_CACHE_SIZE` | `256`                          | Max size of symbols cache (1 entry per process). Change this value if you’re experiencing memory pressure or have many individual services. |

## Sending data to Grafana Cloud or Phlare with Pyroscope eBPF integration

Starting with [weekly-f8](https://hub.docker.com/r/grafana/phlare/tags) you can ingest pyroscope profiles directly to phlare.

```bash
./pyroscope ebpf \
  --application-name=phlare.ebpf.app  \
  --server-address=<URL>              \
  --basic-auth-user="<User>"          \
  --basic-auth-password="<Password>"  \
  --tenant-id=<TenantID>              \
```

To configure eBPF integration to send data to Phlare, replace the `<URL>` placeholder with the appropriate server URL. This could be the Grafana Cloud URL or your own custom Phlare server URL.

If you need to send data to Grafana Cloud, you'll have to configure HTTP Basic authentication. Replace `<User>` with your Grafana Cloud stack user and `<Password>` with your Grafana Cloud API key.

If your Phlare server has multi-tenancy enabled, you'll need to configure a tenant ID. Replace `<TenantID>` with your Phlare tenant ID.

## Examples

Check out the following resources to learn more about eBPF profiling:
- [The pros and cons of eBPF profiling](https://pyroscope.io/blog/ebpf-profiling-pros-cons) blog post (for more context on flamegraphs below)
- [Demo](https://demo.pyroscope.io/?query=rideshare-cluster-ebpf.cpu%7B%7D) showing breakdown of our examples cluster
- [docker-compose example](https://github.com/github/pyroscope/blob/main/examples/ebpf) in our repository
