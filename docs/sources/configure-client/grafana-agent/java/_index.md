---
title: "Profiling Java using the Grafana Agent"
menuTitle: "Profiling Java using the Grafana Agent"
description: "Learn about using Grafana Agent for continuous profiling Java processes for performance optimization."
weight: 20
---

# Profiling Java using the Grafana Agent

Grafana Agent supports Java profiling in [Flow mode](/docs/agent/latest/flow/). Written in the
[River](/docs/agent/latest/flow/config-language/) language, the configuration file is composed of components that are used to collect,
transform, and send data.

## Configure the components

The `pyroscope.java` component is used to continuously profile Java processes running on the local Linux OS
using [async-profiler](https://github.com/async-profiler/async-profiler).

```river
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

Using the `targets` argument, you can specify which processes and containers to profile on the machine. The `targets`
can be from `discovery.process` component. You can use `discovery.process` join argument to join process targets with
extra discoveries such as `dicovery.kubernetes`, `discovery.docker` and `discovery.dockerswarm`.
You can use the `discovery.relabel` component to relabel discovered targets and set your own labels . For more
information, see [Components](/docs/agent/latest/flow/concepts/components/).

The `forward_to` parameter should point to a `pyroscope.write` component to send the collected profiles to your
Pyroscope Server or [Grafana Cloud](/products/cloud/).

| Name         | Type                     | Description                                      | Default | Required |
|--------------|--------------------------|--------------------------------------------------|---------|----------|
| `targets`    | `list(map(string))`      | List of java process targets to profile.         |         | yes      |
| `forward_to` | `list(ProfilesReceiver)` | List of receivers to send collected profiles to. |         | yes      |
| `tmp_dir`    | `string`                 | Temporary directory to store async-profiler.     | `/tmp`  | no       |

The special label `__process_pid__` _must always_ be present in each target of `targets` and corresponds to the `PID` of
the process to profile.

The special label `service_name` is required and must always be present. If `service_name` is not specified, `pyroscope.java`
attempts to infer it from discovery meta labels. If `service_name` is not specified and could not be inferred, then it is
set to `unspecified`.

The `profiling_config` block describes how async-profiler is invoked.

The following arguments are supported:

| Name          | Type       | Description                                                                                              | Default | Required |
|---------------|------------|----------------------------------------------------------------------------------------------------------|---------|----------|
| `interval`    | `duration` | How frequently to collect profiles from the targets.                                                     | "60s"   | no       |
| `cpu`         | `bool`     | A flag to enable cpu profiling, using `itimer` async-profiler event.                                     | true    | no       |
| `sample_rate` | `int`      | CPU profiling sample rate. It is converted from Hz to interval and passed as `-i` arg to async-profiler. | 100     | no       |
| `alloc`       | `string`   | Allocation profiling sampling configuration  It is passed as `--alloc` arg to async-profiler.            | "512k"  | no       |
| `lock`        | `string`   | Lock profiling sampling configuration. It is passed as `--lock` arg to async-profiler.                   | "10ms"  | no       |

For more information on async-profiler configuration,
see [profiler-options](https://github.com/async-profiler/async-profiler?tab=readme-ov-file#profiler-options).

### Set privileges for the Agent

You must run the agent as root and inside host pid namespace for the `pyroscope.java`
and `discover.process` components to work.

### Send data to Grafana Cloud Profiles

When sending to Grafana Cloud Profiles, you can use the following `pyroscope.write` component configuration which uses environment variables.

Ensure that you have appropriately configured the `GC_URL`, `GC_USER`, and `GC_PASSWORD` environment variables.

```river
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

### Profiling local process

```river
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

### Profiling docker containers

```river
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

### Profiling kubernetes pods

```river

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
* [`pyroscope.java`](/docs/agent/next/flow/reference/components/pyroscope.java/)
* [`discovery.process`](/docs/agent/next/flow/reference/components/discovery.process/)
* [`discovery.kubernetes`](/docs/agent/next/flow/reference/components/discovery.kubernetes/)
