---
title: "Setup eBPF Profiling on Docker"
menuTitle: "Setting up on Docker"
description: "Setting up eBPF Profiling with Grafana Alloy on Docker"
weight: 20
---

# Setup eBPF Profiling on Docker

To set up eBPF profiling with Grafana Alloy on Linux, you need to:

- Verify that your system meets the requirements.
- Create an Alloy configuration file. For more information, refer to the [Configuration reference][config-reference].
- Run Alloy.
- Verify that profiles are received.

{{< docs/shared lookup="agent-deprecation.md" source="alloy" version="next" >}}

## Before you begin

Before you begin, you need:

- A Pyroscope server where Alloy can send profiling data.
- Access to Grafana with the [Grafana Pyroscope data source][pyroscope-ds] provisioned.
- [Docker Engine](https://docs.docker.com/engine/install/) installed.

{{% admonition type="note" %}}
If you don't have a Grafana or a Pyroscope server, you can use the [Grafana Cloud][gcloud] free plan to get started.
{{% /admonition %}}

## Verify system requirements

The eBPF profiler requires a Linux kernel version >= 4.9 due to [BPF_PROG_TYPE_PERF_EVENT](https://lkml.org/lkml/2016/9/1/831).

`BPF_PROG_TYPE_PERF_EVENT` is a type of eBPF program that can be attached to hardware or software events, such as performance monitoring counters or tracepoints, in the Linux kernel.

To print the kernel version of your docker host, run:

```shell
docker info | grep Kernel
```

The kernel version must be 4.9 or later.

## Configure Alloy

You can configure Alloy eBPF profiler to profile local containers.
To do so, use the [`discovery.docker` component](https://grafana.com/docs/alloy/<ALLOY_VERSION>/reference/components/discovery/discovery.docker/) to discover local containers and the [`pyroscope.ebpf` component](https://grafana.com/docs/alloy/<ALLOY_VERSION>/reference/components/pyroscope/pyroscope.ebpf/) to profile them

For more information about the Alloy configuration, refer to the [Alloy Components reference](https://grafana.com/docs/alloy/<ALLOY_VERSION>/reference/components/).

Create a file named `alloy.config` with the following content:

```alloy
discovery.docker "local_containers" {
 host = "unix:///var/run/docker.sock"
}

pyroscope.ebpf "instance" {
 forward_to     = [pyroscope.write.endpoint.receiver]
 targets = discovery.docker.local_containers.targets
}

pyroscope.write "endpoint" {
 endpoint {
  basic_auth {
   password = "<PASSWORD>"
   username = "<USERNAME>"
  }
  url = "<URL>"
 }
 external_labels = {
  "env"      = "testing",
  "instance" = env("HOSTNAME"),
 }
}
```

Replace the `<URL>` placeholder with the appropriate server URL.
This could be the Grafana Cloud URL or your own custom Pyroscope server URL.

If you need to send data to Grafana Cloud, you'll have to configure HTTP Basic authentication.
Replace `<User>` with your Grafana Cloud stack user and `<Password>` with your Grafana Cloud API key.

For more information, refer to the [Configure the Grafana Pyroscope data source documentation](https://grafana.com/docs/grafana-cloud/connect-externally-hosted/data-sources/pyroscope/configure-pyroscope-data-source/).

{{% admonition type="note" %}}
If you're using your own Pyroscope server, you can remove the `basic_auth` section altogether.
{{% /admonition %}}

## Start Alloy

To start Alloy with Docker, run:

```shell
docker run \
  -v $PWD/alloy.config:/etc/alloy/alloy.config \
  -v /var/run/docker.sock:/var/run/docker.sock \
  --pid=host \
  --privileged \
  -p 12345:12345 \
  grafana/alloy:latest \
    run --server.http.listen-addr=0.0.0.0:12345 /etc/alloy/alloy.config
```

{{< admonition type="note" >}}
The `--pid=host` and `--privileged` flags are required to profile local containers with eBPF.
{{< /admonition >}}

## Verify profiles are received

To verify that the profiles are received by the Pyroscope server, go to the Pyroscope UI or [Grafana Pyroscope data source][pyroscope-ds]. Then select a profile type and a service from the dropdown menu.

[pyroscope-ds]: /docs/grafana/<GRAFANA_VERSION>/datasources/pyroscope/
[config-reference]: ../configuration/
[gcloud]: /products/cloud/
