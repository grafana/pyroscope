---
title: "Setup eBPF Profiling on Docker"
menuTitle: "Setting up on Docker"
description: "Setting up eBPF Profiling with Grafana Agent on Docker"
weight: 20
---

# Setup eBPF Profiling on Docker

To set up eBPF profiling with Grafana Agent on Linux, you need to:

- Verify that your system meets the requirements.
- Create a Grafana Agent configuration file. For more information, see [Configuration reference][config-reference].
- Run the Grafana Agent.
- Finally, verify that profiles are received.

## Prerequisites

Before you begin, you need:

- A Pyroscope server where the agent will send profiling data.
- Access to Grafana with the [Grafana Pyroscope datasource][pyroscope-ds] provisioned.
- [Docker Engine](https://docs.docker.com/engine/install/) installed.

{{% admonition type="note" %}}
If you don't have a Grafana and/or a Pyroscope server, you can use the [Grafana Cloud][gcloud] free plan to get started.
{{% /admonition %}}

## Verify system meets the requirements

The eBPF profiler requires a Linux kernel version >= 4.9 (due to [BPF_PROG_TYPE_PERF_EVENT](https://lkml.org/lkml/2016/9/1/831)).

`BPF_PROG_TYPE_PERF_EVENT` is a type of eBPF program that can be attached to hardware or software events, such as performance monitoring counters or tracepoints, in the Linux kernel.

To print the kernel version of your docker host, run:

```shell
docker info | grep Kernel
```

Make sure you have a kernel version >= 4.9.

## Configure the Grafana Agent

We'll configure the Grafana Agent eBPF profiler to profile local containers. To do so we'll use the `discovery.docker` component to discover local containers and the `pyroscope.ebpf` component to profile them.

Create a file named `agent.river` with the following content:

```river
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

Replace the `<URL>` placeholder with the appropriate server URL. This could be the Grafana Cloud URL or your own custom Pyroscope server URL.

If you need to send data to Grafana Cloud, you'll have to configure HTTP Basic authentication. Replace `<User>` with your Grafana Cloud stack user and `<Password>` with your Grafana Cloud API key.

{{% admonition type="note" %}}
If you're using your own Pyroscope server, you can remove the `basic_auth` section altogether.
{{% /admonition %}}

For more information, refer to the [Configure the Grafana Pyroscope data source documentation](/docs/grafana-cloud/connect-externally-hosted/data-sources/grafana-pyroscope#configure-the-grafana-pyroscope-data-source).

## Start the Grafana Agent

To start the Grafana Agent with docker, run:

```shell
docker run \
  -e AGENT_MODE=flow \
  -v $PWD/agent.river:/etc/agent/config.river \
  -v /var/run/docker.sock:/var/run/docker.sock \
  --pid=host \
  --privileged \
  -p 12345:12345 \
  grafana/agent:latest \
    run --server.http.listen-addr=0.0.0.0:12345 /etc/agent/config.river
```

> Note: The `--pid=host` and `--privileged` flags are required to profile local containers with ebpf.

## Verify profiles are received

To verify that the profiles are received by the Pyroscope server, go to the Pyroscope UI or [Grafana Pyroscope datasource][pyroscope-ds]. Then select a profile type and a service from the dropdown menu.

[pyroscope-ds]: /docs/grafana/latest/datasources/grafana-pyroscope/
[config-reference]: ../configuration/
[gcloud]: /products/cloud/
