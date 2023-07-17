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
helm repo add grafana https://grafana.github.io/helm-charts
helm repo update
```

### Step 2: Install pyroscope agent

```yaml
agent:
  mode: 'flow'
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
```
Replace the `<URL>` placeholder with the appropriate server URL. This could be the Grafana Cloud URL or your own custom Phlare server URL.

If you need to send data to Grafana Cloud, you'll have to configure HTTP Basic authentication. Replace `<User>` with your Grafana Cloud stack user and `<Password>` with your Grafana Cloud API key.

```shell
helm install pyroscope-ebpf grafana/grafana-agent -f values.yaml
```

It will install pyroscope eBPF agent on all of your nodes and start profiling applications across your cluster.

## Configuration

The component configures and starts a new ebpf profiling job to collect performance profiles from the current host.

You can use the following arguments to configure a `pyroscope.ebpf`. Only the
`forward_to` and `targets` fields are required. Omitted fields take their default
values.

| Name                      | Type                     | Description                                                  | Default | Required |
|---------------------------|--------------------------|--------------------------------------------------------------|---------|----------|
| `targets`                 | `list(map(string))`      | List of targets to group profiles by container id            |         | yes      |
| `forward_to`              | `list(ProfilesReceiver)` | List of receivers to send collected profiles to.             |         | yes      |   
| `collect_interval`        | `duration`               | How frequently to collect profiles                           | `15s`   | no       |       
| `sample_rate`             | `int`                    | How many times per second to collect profile samples         | 97      | no       |     
| `pid_cache_size`          | `int`                    | The size of the pid -> proc symbols table LRU cache          | 32      | no       |      
| `build_id_cache_size`     | `int`                    | The size of the elf file build id -> symbols table LRU cache | 64      | no       |       
| `same_file_cache_size`    | `int`                    | The size of the elf file -> symbols table LRU cache          | 8       | no       |       
| `container_id_cache_size` | `int`                    | The size of the pid -> container ID table LRU cache          | 1024    | no       |       
| `collect_user_profile`    | `bool`                   | A flag to enable/disable collection of userspace profiles    | true    | no       |       
| `collect_kernel_profile`  | `bool`                   | A flag to enable/disable collection of kernelspace profiles  | true    | no       |       

## Exported fields

`pyroscope.ebpf` does not export any fields that can be referenced by other
components.

## Component health

`pyroscope.ebpf` is only reported as unhealthy if given an invalid
configuration.

## Debug information

* `targets` currently tracked active targets.
* `pid_cache` per process elf symbol tables and their sizes in symbols count.
* `elf_cache` per build id and per same file symbol tables and their sizes in symbols count.

## Debug metrics

* `pyroscope_fanout_latency` (histogram): Write latency for sending to direct and indirect components.
* `pyroscope_ebpf_active_targets` (gauge): Number of active targets the component tracks.
* `pyroscope_ebpf_profiling_sessions_total` (counter): Number of profiling sessions completed.
* `pyroscope_ebpf_profiling_sessions_failing_total` (counter): Number of profiling sessions failed.
* `pyroscope_ebpf_pprofs_total` (counter): Number of pprof profiles collected by the ebpf component.

## Profile collecting behavior

The `pyroscope.ebpf` component collects stack traces associated with a process running on the current host.
You can use the `sample_rate` argument to define the number of stack traces collected per second. The default is 97.

The following labels are automatically injected into the collected profiles if you have not defined them. These labels
can help you pin down a profiling target.

| Label              | Description                                                                                                                      |
|--------------------|----------------------------------------------------------------------------------------------------------------------------------|
| `service_name`     | Pyroscope service name. It's automatically selected from discovery meta labels if possible. Otherwise defaults to `unspecified`. |
| `__name__`         | pyroscope metric name. Defaults to `process_cpu`.                                                                                |
| `__container_id__` | The container ID derived from target.                                                                                            |

### Container ID

Each collected stack trace is then associated with a specified target from the targets list, determined by a
container ID. This association process involves checking the `__container_id__`, `__meta_docker_container_id`,
and `__meta_kubernetes_pod_container_id` labels of a target against the `/proc/{pid}/cgroup` of a process.

If a corresponding container ID is found, the stack traces are aggregated per target based on the container ID.
If a container ID is not found, the stack trace is associated with a `default_target`.

Any stack traces not associated with a listed target are ignored.

### Service name

The special label `service_name` is required and must always be present. If it's not specified, it is
attempted to be inferred from multiple sources:

- `__meta_kubernetes_pod_annotation_pyroscope_io_service_name` which is a `pyroscope.io/service_name` pod annotation.
- `__meta_kubernetes_namespace` and `__meta_kubernetes_pod_container_name`
- `__meta_docker_container_name`

If `service_name` is not specified and could not be inferred, it is set to `unspecified`.

## Troubleshooting unknown symbols

Symbols are extracted from various sources, including:

- The `.symtab` and `.dynsym` sections in the ELF file.
- The `.symtab` and `.dynsym` sections in the debug ELF file.
- The `.gopclntab` section in Go language ELF files.

The search for debug files follows [gdb algorithm](https://sourceware.org/gdb/onlinedocs/gdb/Separate-Debug-Files.html).
For example, if the profiler wants to find the debug file
for `/lib/x86_64-linux-gnu/libc.so.6`
with a `.gnu_debuglink` set to `libc.so.6.debug` and a build ID `0123456789abcdef`. The following paths are examined:

- `/usr/lib/debug/.build-id/01/0123456789abcdef.debug`
- `/lib/x86_64-linux-gnu/libc.so.6.debug`
- `/lib/x86_64-linux-gnu/.debug/libc.so.6.debug`
- `/usr/lib/debug/lib/x86_64-linux-gnu/libc.so.6.debug`

### Dealing with unknown symbols

Unknown symbols in the profiles you’ve collected indicate that the profiler couldn't access an ELF file ￼associated with a given address in the trace.

This can occur for several reasons:

- The process has terminated, making the ELF file inaccessible.
- The ELF file is either corrupted or not recognized as an ELF file.
- There is no corresponding ELF file entry in `/proc/pid/maps` for the address in the stack trace.

### Addressing unresolved symbols

If you only see module names (e.g., `/lib/x86_64-linux-gnu/libc.so.6`) without corresponding function names, this
indicates that the symbols couldn't be mapped to their respective function names.

This can occur for several reasons:

- The binary has been stripped, leaving no .symtab, .dynsym, or .gopclntab sections in the ELF file.
- The debug file is missing or could not be located.

To fix this for your binaries, ensure that they are either not stripped or that you have separate
debug files available. You can achieve this by running:

```bash
objcopy --only-keep-debug elf elf.debug
strip elf -o elf.stripped
objcopy --add-gnu-debuglink=elf.debug elf.stripped elf.debuglink
```

For system libraries, ensure that debug symbols are installed. On Ubuntu, for example, you can install them by
executing:

```bash
apt install libc6-dbg
```

### Understanding flat stack traces

If your profiles show many shallow stack traces, typically 1-2 frames deep, your binary might have been compiled without frame pointers.

To compile your code with frame pointers, include the `-fno-omit-frame-pointer` flag in your compiler options.

### Profiling interpreted languages

Profiling interpreted languages like Python, Ruby, JavaScript, etc., is not ideal using this implementation.
The JIT-compiled methods in these languages are typically not in ELF file format, demanding additional steps for
profiling. For instance, using perf-map-agent and enabling frame pointers for Java.

Interpreted methods will display the interpreter function’s name rather than the actual function.

## Examples

Check out the following resources to learn more about eBPF profiling:
- [The pros and cons of eBPF profiling](https://pyroscope.io/blog/ebpf-profiling-pros-cons) blog post (for more context on flamegraphs below)
- [Demo](https://demo.pyroscope.io/?query=rideshare-cluster-ebpf.cpu%7B%7D) showing breakdown of our examples cluster
- Grafana agent documnetation for [pyroscope.ebpf](/docs/agent/next/flow/reference/components/pyroscope.ebpf/), [pyroscope.write](/docs/agent/next/flow/reference/components/pyroscope.write/), [discovery.kubernetes](/docs/agent/next/flow/reference/components/discovery.kubernetes/), [discovery.relabel](/docs/agent/next/flow/reference/components/discovery.relabel/) components
