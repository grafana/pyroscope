---
title: "Setup eBPF Profiling on Kubernetes"
menuTitle: "Setting up on Kubernetes"
description: "Setting up eBPF Profiling with Grafana Alloy on Kubernetes"
weight: 10
---

# Setup eBPF Profiling on Kubernetes

To set up eBPF profiling with Grafana Alloy on Kubernetes, you need to:

- Verify that your cluster meets the prerequisites.
- Add the Grafana helm repository.
- Create an Alloy configuration file. For more information, refer to [Configuration reference][config-reference].
- Install Alloy, refer to the [installation instructions](https://grafana.com/docs/alloy/<ALLOY_VERSION>/set-up/install/kubernetes/)
- Verify that profiles are received.

## Before you begin

Before you begin, you need:

- [Helm][helm] and [kubectl][kubectl] installed with access to your Kubernetes cluster.
- A Pyroscope server where Alloy can send profiling data.
- Access to Grafana with the [Grafana Pyroscope data source][pyroscope-ds] provisioned.

{{< admonition type="note" >}}
If you don't have a Grafana or a Pyroscope server, you can use the [Grafana Cloud][gcloud] free plan to get started.
{{< /admonition >}}

## Verify that your cluster meets the requirements

The eBPF profiler requires a Linux kernel version >= 4.9 (due to [BPF_PROG_TYPE_PERF_EVENT](https://lkml.org/lkml/2016/9/1/831)).

`BPF_PROG_TYPE_PERF_EVENT` is a type of eBPF program that can be attached to hardware or software events, such as performance monitoring counters or tracepoints, in the Linux kernel.

To print the kernel version of each node in your cluster, run:

```shell
kubectl get nodes -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.nodeInfo.kernelVersion}{"\n"}{end}'
```

Make sure all nodes have a kernel version >= 4.9.

## Add the Grafana Helm repository

Use [Helm][helm] to install Alloy.
To add the Grafana Helm repository, run:

```shell
helm repo add grafana https://grafana.github.io/helm-charts
helm repo update
```

Verify that the repository was added successfully by running:

```shell
helm search repo grafana/alloy
```

The command returns a list of available versions of Alloy.

## Create an Alloy configuration file

Create a file named `values.yaml` with the content from the sample configuration file.

```yaml
alloy:
  configMap:
    create: true
    content: |
      discovery.kubernetes "local_pods" {
        selectors {
          field = "spec.nodeName=" + env("HOSTNAME")
          role = "pod"
        }
        role = "pod"
      }
      pyroscope.ebpf "instance" {
        forward_to = [pyroscope.write.endpoint.receiver]
        targets = discovery.kubernetes.local_pods.targets
      }
      pyroscope.write "endpoint" {
        endpoint {
          basic_auth {
            password = "<PASSWORD>"
            username = "<USERNAME>"
          }
          url = "<URL>"
        }
      }

  securityContext:
    privileged: true
    runAsGroup: 0
    runAsUser: 0

controller:
  hostPID: true
```

For information about configuring Alloy, refer to [Grafana Alloy on Kubernetes](https://grafana.com/docs/alloy/<ALLOY_VERSION>/configure/kubernetes/).

For information about the specific blocks used, refer to the [Grafana Alloy Reference](https://grafana.com/docs/alloy/<ALLOY_VERSION>/reference/).

Replace the `<URL>` placeholder with the appropriate server URL.
This could be the Grafana Cloud URL or your own custom Pyroscope server URL.

If you need to send data to Grafana Cloud, you'll have to configure HTTP Basic authentication.
Replace `<User>` with your Grafana Cloud stack user and `<Password>` with your Grafana Cloud API key.

For more information, refer to the [Configure the Grafana Pyroscope data source documentation](/docs/grafana-cloud/connect-externally-hosted/data-sources/pyroscope/configure-pyroscope-data-source/).

{{% admonition type="note" %}}
If you're using your own Pyroscope server, you can remove the `basic_auth` section altogether.
{{% /admonition %}}


## Install Alloy

To install Alloy, run:

```shell
helm install pyroscope-ebpf grafana/alloy -f values.yaml
```

Once configured, Alloy starts collecting eBPF profiles and sends them to the Pyroscope server.

## Verify profiles are received

To verify that the profiles are received by the Pyroscope server:

1. Go to the Pyroscope UI or [Grafana Pyroscope data source][pyroscope-ds].
1. Select a profile type and a service from the drop-down menu.

[gcloud]: /products/cloud/
[helm]: https://helm.sh/docs/intro/install/
[kubectl]: https://kubernetes.io/docs/tasks/tools/install-kubectl/
[pyroscope-ds]: /docs/grafana/<GRAFANA_VERSION>/datasources/pyroscope/
[config-reference]: ../configuration/
