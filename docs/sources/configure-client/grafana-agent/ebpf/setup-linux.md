---
title: "Set up eBPF profiling on Linux"
menuTitle: "Set up on Linux"
description: "Set up eBPF profiling with Grafana Agent on Linux machines."
weight: 20
---

# Set up eBPF profiling on Linux

To set up eBPF profiling with Grafana Agent on Linux, you need to:

- Verify that your system meets the requirements.
- Install the Grafana Agent Flow mode.
- Create a Grafana Agent configuration file. For more information, see [Configuration reference][config-reference].
- Run the Grafana Agent.
- Finally, verify that profiles are received.

## Prerequisites

Before you begin, you need:

- A Pyroscope server where the agent will send profiling data.
- Access to Grafana with the [Grafana Pyroscope datasource][pyroscope-ds] provisioned.

{{% admonition type="note" %}}
If you don't have a Grafana and/or a Pyroscope server, you can use the [Grafana Cloud][gcloud] free plan to get started.
{{% /admonition %}}

## Verify system meets the requirements

The eBPF profiler requires a Linux kernel version >= 4.9 (due to [BPF_PROG_TYPE_PERF_EVENT](https://lkml.org/lkml/2016/9/1/831)).

`BPF_PROG_TYPE_PERF_EVENT` is a type of eBPF program that can be attached to hardware or software events, such as performance monitoring counters or tracepoints, in the Linux kernel.

To print the kernel version of machine, run:

```shell
uname -r
```

Make sure you have a kernel version >= 4.9.

## Install the Grafana Agent

Follow the [installation instructions][agent-install] to download and install the Grafana Agent for your current Linux distribution.

Verify that the agent is correctly installed by running:

```shell
grafana-agent-flow --version
```

## Configure the Grafana Agent

To configure the Grafana Agent eBPF profiler to profile local processes, you'll need to set the `targets_only` flag to `false` and add a default target in the `pyroscope.ebpf` component.
All processes will be profiled and grouped under the default target.

{{% admonition type="note" %}}
We're [working on a more flexible configuration](https://github.com/grafana/agent/pull/5858) that will allow you to specify which processes to profile.
{{% /admonition %}}

Create a file named `agent.river` with the following content:

```river
discovery.process "all" {

}

discovery.relabel "agent" {
    targets = discovery.process.all.targets
    // Filter needed processes
    rule {
        source_labels = ["__meta_process_exe"]
        regex = ".*/grafana-agent"
        action = "keep"
    }
}

pyroscope.ebpf "instance" {
 forward_to     = [pyroscope.write.endpoint.receiver]
 targets = discovery.relabel.agent.output
}

pyroscope.scrape "local" {
  forward_to     = [pyroscope.write.endpoint.receiver]
  targets    = [
    {"__address__" = "localhost:12345", "service_name"="grafana/agent"},
  ]
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
  "env"      = "prod",
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

To start the Grafana Agent, run:

```shell
grafana-agent-flow run agent.river
```

If you see the following error:

```shell
level=error msg="component exited with error" component=pyroscope.ebpf.local_pods err="ebpf profiling session start: load bpf objects: field DisassociateCtty: program disassociate_ctty: map events: map create: operation not permitted (MEMLOCK may be too low, consider rlimit.RemoveMemlock)"
```

Make sure you're running the agent with root privileges which are required for the eBPF profiler to work.

## Verify profiles are received

To verify that the profiles are received by the Pyroscope server, go to the Pyroscope UI or [Grafana Pyroscope datasource][pyroscope-ds]. Select a profile type and a service from the drop-down menu.

[agent-install]: /docs/agent/latest/flow/setup/install/linux/
[pyroscope-ds]: /docs/grafana/latest/datasources/grafana-pyroscope/
[config-reference]: ../configuration/
[gcloud]: /products/cloud/
[discovery.process](/docs/agent/next/flow/reference/components/discovery.process/)
