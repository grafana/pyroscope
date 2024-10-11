---
title: "Profiling Java using Grafana Alloy"
menuTitle: "Profiling Java using Alloy"
description: "Learn about using Grafana Alloy for continuous profiling Java processes for performance optimization."
weight: 20
---

# Profiling Java using Grafana Alloy

Grafana Alloy supports Java profiling.
The collector configuration file is composed of components that are used to collect,
transform, and send data.
The Alloy configuration files use the Alloy [configuration syntax](https://grafana.com/docs/alloy/<ALLOY_VERSION>/concepts/configuration-syntax/).

## Configure the components

The [`pyroscope.java` component](https://grafana.com/docs/alloy/<ALLOY_VERSION>/reference/components/pyroscope/pyroscope.java/) is used to continuously profile Java processes running on the local Linux OS
using [async-profiler](https://github.com/async-profiler/async-profiler).

```alloy
pyroscope.java "java" {
  profiling_config {
    interval = "15s"
    alloc = "512k"
    cpu = true
    lock = "10ms"
    sample_rate = 100
  }
  forward_to = [pyroscope.write.endpoint.receiver]
  targets = discovery.relabel.java.output
}
```

Using the `targets` argument, you can specify which processes and containers to profile on the machine.

The `targets` can be from `discovery.process` component.
You can use `discovery.process` join argument to join process targets with extra discoveries such as [`discovery.kubernetes`](https://grafana.com/docs/alloy/<ALLOY_VERSION>/reference/components/discovery/discovery.kubernetes/), [`discovery.docker`](https://grafana.com/docs/alloy/<ALLOY_VERSION>/reference/components/discovery/discovery.docker/), and [`discovery.dockerswarm`](https://grafana.com/docs/alloy/<ALLOY_VERSION>/reference/components/discovery/discovery.dockerswarm/).
You can use the [`discovery.relabel`](https://grafana.com/docs/alloy/<ALLOY_VERSION>/reference/components/discovery/discovery.relabel/) component to relabel discovered targets and set your own labels.
For more information, refer to the [Components](https://grafana.com/docs/alloy/<ALLOY_VERSION>/reference/components/) documentation.

The `forward_to` parameter should point to a [`pyroscope.write`](https://grafana.com/docs/alloy/<ALLOY_VERSION>/reference/components/pyroscope/pyroscope.write/) component to send the collected profiles to your
Pyroscope Server or [Grafana Cloud](/products/cloud/).

| Name         | Type                     | Description                                      | Default | Required |
|--------------|--------------------------|--------------------------------------------------|---------|----------|
| `targets`    | `list(map(string))`      | List of java process targets to profile.         |         | yes      |
| `forward_to` | `list(ProfilesReceiver)` | List of receivers to send collected profiles to. |         | yes      |
| `tmp_dir`    | `string`                 | Temporary directory to store async-profiler.     | `/tmp`  | no       |

The special label `__process_pid__` _must always_ be present in each target of `targets` and corresponds to the `PID` of
the process to profile.

The special label `service_name` is required and must always be present.
If `service_name` isn't specified, `pyroscope.java` attempts to infer it from discovery meta labels.
If `service_name` isn't specified and couldn't be inferred, then it's set to `unspecified`.

The `profiling_config` block describes how async-profiler is invoked.

It supports the following arguments:

| Name          | Type       | Description                                                                                              | Default | Required |
|---------------|------------|----------------------------------------------------------------------------------------------------------|---------|----------|
| `interval`    | `duration` | How frequently to collect profiles from the targets.                                                     | "60s"   | no       |
| `cpu`         | `bool`     | A flag to enable CPU profiling, using `itimer` async-profiler event.                                     | true    | no       |
| `sample_rate` | `int`      | CPU profiling sample rate. It's converted from Hz to interval and passed as `-i` arg to async-profiler. | 100     | no       |
| `alloc`       | `string`   | Allocation profiling sampling configuration  It's passed as `--alloc` arg to async-profiler.            | "512k"  | no       |
| `lock`        | `string`   | Lock profiling sampling configuration. It's passed as `--lock` arg to async-profiler.                   | "10ms"  | no       |

For more information on async-profiler configuration,
see [profiler-options](https://github.com/async-profiler/async-profiler?tab=readme-ov-file#profiler-options).

### Set privileges for the collector

You must run the collector, such Alloy, as root and inside host `pid` namespace for the `pyroscope.java` and `discover.process` components to work.

### Start the collector

To start Grafana Alloy v1.2 and later, replace `configuration.alloy` with your configuration filename:

```bash
alloy run configuration.alloy
```

To start Grafana Alloy v1.0 or 1.1, replace `configuration.alloy` with your configuration file name:

```bash
alloy run --stability.level=public-preview configuration.alloy
```

The `stability.level` option is required for `pyroscope.scrape` with Alloy v1.0 or v1.1. For more information about `stability.level`, refer to [the run command](https://grafana.com/docs/alloy/<ALLOY_VERSION>/reference/cli/run/#permitted-stability-levels) documentation.


### Send data to Grafana Cloud Profiles

When sending to Grafana Cloud Profiles, you can use the following `pyroscope.write` component configuration which uses environment variables.

Ensure that you have appropriately configured the `GC_URL`, `GC_USER`, and `GC_PASSWORD` environment variables.

```alloy
pyroscope.write "endpoint" {
    endpoint {
        basic_auth {
            password = env("GC_PASSWORD")
            username = env("GC_USER")
        }
        url = env("GC_URL")
    }
}
```

## Examples

For more robust examples, refer to the [Grafana Alloy and Agent Auto-instrumentation](https://github.com/grafana/pyroscope/tree/main/examples/grafana-agent-auto-instrumentation) examples in the Pyroscope repository.

### Profiling local process

```alloy
discovery.process "all" {
}

discovery.relabel "java" {
    targets = discovery.process.all.targets
    // Filter only java processes
    rule {
        source_labels = ["__meta_process_exe"]
        action = "keep"
        regex = ".*/java$"
    }
    // Filter processes. For example: only processes with command line containing "FastSlow"
    rule {
        source_labels = ["__meta_process_commandline"]
        regex = "java FastSlow"
        action = "keep"
    }
    // Provide a service name for the process, otherwise it will be unspecified.
    rule {
        action = "replace"
        target_label = "service_name"
        replacement = "java-fast-slow"
    }
}

pyroscope.java "java" {
  forward_to = [pyroscope.write.example.receiver]
  targets = discovery.relabel.java.output
}

pyroscope.write "example" {
  endpoint {
    url = "http://pyroscope:4040"
  }
}

```

### Profiling Docker containers

For a working example, refer to [Java profiling via auto-instrumentation example in Docker](https://github.com/grafana/pyroscope/tree/main/examples/grafana-agent-auto-instrumentation/java/docker).

```alloy
discovery.docker "local_containers" {
  host = "unix:///var/run/docker.sock"
}

discovery.process "all" {
  join = discovery.docker.local_containers.targets
}

discovery.relabel "java" {
    targets = discovery.process.all.targets
    // Filter only java processes
    rule {
        source_labels = ["__meta_process_exe"]
        action = "keep"
        regex = ".*/java$"
    }
    // Filter only needed containers
    rule {
        source_labels = ["__meta_docker_container_name"]
        regex = ".*suspicious_pascal"
        action = "keep"
    }
    // Provide a service name for the process, otherwise it will default to the value of __meta_docker_container_name label.
    rule {
        action = "replace"
        target_label = "service_name"
        replacement = "java-fast-slow"
    }
}

pyroscope.java "java" {
  forward_to = [pyroscope.write.example.receiver]
  targets = discovery.relabel.java.output
}

pyroscope.write "example" {
  endpoint {
    url = "http://pyroscope:4040"
  }
}
```

### Profiling Kubernetes pods

For a working example, refer to [Grafana Alloy Java profiling via auto-instrumentation with Kubernetes](https://github.com/grafana/pyroscope/tree/main/examples/grafana-agent-auto-instrumentation/java/kubernetes).

```alloy
discovery.kubernetes "local_pods" {
  selectors {
    field = "spec.nodeName=" + env("HOSTNAME")
    role = "pod"
  }
  role = "pod"
}

discovery.process "all" {
  join = discovery.kubernetes.local_pods.targets
}

discovery.relabel "java_pods" {
  targets = discovery.process.all.targets
  // Filter only java processes
  rule {
    source_labels = ["__meta_process_exe"]
    action = "keep"
    regex = ".*/java$"
  }
  rule {
    action = "drop"
    regex = "Succeeded|Failed|Completed"
    source_labels = ["__meta_kubernetes_pod_phase"]
  }
  rule {
    action = "replace"
    source_labels = ["__meta_kubernetes_namespace"]
    target_label = "namespace"
  }
  rule {
    action = "replace"
    source_labels = ["__meta_kubernetes_pod_name"]
    target_label = "pod"
  }
  rule {
    action = "replace"
    source_labels = ["__meta_kubernetes_pod_node_name"]
    target_label = "node"
  }
  rule {
    action = "replace"
    source_labels = ["__meta_kubernetes_pod_container_name"]
    target_label = "container"
  }
  // Provide arbitrary service_name label, otherwise it will be inferred from discovery labels automatically
  rule {
    action = "replace"
    regex = "(.*)@(.*)"
    replacement = "java/${1}/${2}"
    separator = "@"
    source_labels = ["__meta_kubernetes_namespace", "__meta_kubernetes_pod_container_name"]
    target_label = "service_name"
  }
  // Filter only needed services
  rule {
    action = "keep"
    regex = "(java/ns1/.*)|(java/ns2/container-.*0)"
    source_labels = ["service_name"]
  }
}

pyroscope.java "java" {
  forward_to = [pyroscope.write.example.receiver]
  targets = discovery.relabel.java.output
}

pyroscope.write "example" {
  endpoint {
    url = "http://pyroscope:4040"
  }
}
```

## References

For more information:

* [Examples](https://github.com/grafana/pyroscope/tree/main/examples/grafana-agent-auto-instrumentation/java)

- [Grafana Alloy](https://grafana.com/docs/alloy/<ALLOY_VERSION>/)
- [pyroscope.scrape](https://grafana.com/docs/alloy/<ALLOY_VERSION>/reference/components/pyroscope/pyroscope.scrape/)
- [pyroscope.write](https://grafana.com/docs/alloy/<ALLOY_VERSION>/reference/components/pyroscope/pyroscope.write/)
- [discovery.kubernetes](https://grafana.com/docs/alloy/<ALLOY_VERSION>/reference/components/discovery/discovery.kubernetes/)
- [discovery.docker](https://grafana.com/docs/alloy/<ALLOY_VERSION>/reference/components/discovery/discovery.docker/)
- [discovery.relabel](https://grafana.com/docs/alloy/<ALLOY_VERSION>/reference/components/discovery/discovery.relabel/)
