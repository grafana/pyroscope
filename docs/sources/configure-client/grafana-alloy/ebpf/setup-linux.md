---
title: "Set up eBPF profiling on Linux"
menuTitle: "Set up on Linux"
description: "Set up eBPF profiling with Grafana Alloy on Linux machines."
weight: 20
---

# Set up eBPF profiling on Linux

To set up eBPF profiling with Grafana Alloy on Linux, you need to:

- Verify that your system meets the requirements.
- Install [Alloy](https://grafana.com/docs/alloy/<ALLOY_VERSION>/set-up/install/linux/).
- Create an [Alloy configuration file](https://grafana.com/docs/alloy/<ALLOY_VERSION>/configure/linux/). For more information, refer to [Configuration reference](https://grafana.com/docs/alloy/<ALLOY_VERSION>/reference/).
- Run Alloy.
- Finally, verify that profiles are received.

## Before you begin

Before you begin, you need:

- A Pyroscope server where Alloy can send profiling data.
- Access to Grafana with the [Grafana Pyroscope data source][pyroscope-ds] provisioned.

{{% admonition type="note" %}}
If you don't have a Grafana or a Pyroscope server, you can use the [Grafana Cloud][gcloud] free plan to get started.
{{% /admonition %}}

## Verify system meets the requirements

The eBPF profiler requires a Linux kernel version >= 4.9 (due to [BPF_PROG_TYPE_PERF_EVENT](https://lkml.org/lkml/2016/9/1/831)).

`BPF_PROG_TYPE_PERF_EVENT` is a type of eBPF program that can be attached to hardware or software events, such as performance monitoring counters or tracepoints, in the Linux kernel.

To print the kernel version of machine, run:

```shell
uname -r
```

Make sure you have a kernel version >= 4.9.

## Install Alloy

Follow the [installation instructions](https://grafana.com/docs/alloy/<ALLOY_VERSION>/set-up/install/linux/) to download and install Alloy for your current Linux distribution.

## Configure Alloy

To configure the Alloy eBPF profiler to profile local processes, use [discovery.process component](https://grafana.com/docs/alloy/<ALLOY_VERSION>/reference/components/discovery/discovery.process/) and add a default target in the [`pyroscope.ebpf` component](https://grafana.com/docs/alloy/<ALLOY_VERSION>/reference/components/pyroscope/pyroscope.ebpf/).
All processes are profiled and grouped under the default target.

Create a file named `alloy.config` with the following content:

```alloy
discovery.process "all" {

}

discovery.relabel "alloy" {
    targets = discovery.process.all.targets
    // Filter needed processes
    rule {
        source_labels = ["__meta_process_exe"]
        regex = ".*/alloy"
        action = "keep"
    }
}

pyroscope.ebpf "instance" {
 forward_to     = [pyroscope.write.endpoint.receiver]
 targets = discovery.relabel.alloy.output
}

pyroscope.scrape "local" {
  forward_to     = [pyroscope.write.endpoint.receiver]
  targets    = [
    {"__address__" = "localhost:12345", "service_name"="grafana/alloy"},
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

For information about configuring Alloy, refer to [Grafana Alloy on Kubernetes](https://grafana.com/docs/alloy/<ALLOY_VERSION>/configure/kubernetes/).

For information about the specific blocks used, refer to the [Grafana Alloy Reference](https://grafana.com/docs/alloy/<ALLOY_VERSION>/reference/) and [`discovery.process`](https://grafana.com/docs/alloy/<AlLOY_VERSION>/reference/components/discovery/discovery.process/).

Replace the `<URL>` placeholder with the appropriate server URL. This could be the Grafana Cloud URL or your own custom Pyroscope server URL.

If you need to send data to Grafana Cloud, you'll have to configure HTTP Basic authentication. Replace `<User>` with your Grafana Cloud stack user and `<Password>` with your Grafana Cloud API key.

For more information, refer to the [Configure the Grafana Pyroscope data source documentation](/docs/grafana-cloud/connect-externally-hosted/data-sources/pyroscope/configure-pyroscope-data-source/).

{{% admonition type="note" %}}
If you're using your own Pyroscope server, you can remove the `basic_auth` section altogether.
{{% /admonition %}}

## Start Alloy

{{< admonition type="note">}}
The eBPF profiler requires root privileges.
{{< /admonition >}}

To start the Alloy, run:

```shell
alloy run alloy.config
```

If you see the following error:

```shell
level=error msg="component exited with error" component=pyroscope.ebpf.local_pods err="ebpf profiling session start: load bpf objects: field DisassociateCtty: program disassociate_ctty: map events: map create: operation not permitted (MEMLOCK may be too low, consider rlimit.RemoveMemlock)"
```

Make sure you're running Alloy with root privileges which are required for the eBPF profiler to work.

## Verify profiles are received

To verify that the profiles are received by the Pyroscope server, go to the Pyroscope UI or [Grafana Pyroscope data source][pyroscope-ds]. Select a profile type and a service from the drop-down menu.

[pyroscope-ds]: /docs/grafana/<GRAFANA_VERSION>/datasources/pyroscope/
[config-reference]: ../configuration/
[gcloud]: /products/cloud/
