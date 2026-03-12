---
title: OpenTelemetry eBPF profiler
menuTitle: OpenTelemetry eBPF profiler
description: Collect system-wide continuous profiles using the OpenTelemetry eBPF profiler and send them to Pyroscope.
weight: 150
---

# OpenTelemetry eBPF profiler

Pyroscope supports receiving profiles from the [OpenTelemetry eBPF profiler](https://github.com/open-telemetry/opentelemetry-ebpf-profiler) via OTLP.
The eBPF profiler collects system-wide CPU profiles from all processes on a Linux host and sends them through the OpenTelemetry Collector pipeline to Pyroscope.

## Before you begin

Consider the following limitations:

- **Protocol stability**: The [OpenTelemetry profiles signal](https://github.com/open-telemetry/opentelemetry-proto/tree/main/opentelemetry/proto/profiles) is under active development. Breaking changes have occurred and may continue. Compatibility between the profiler, collector, and Pyroscope requires careful version management.
- **Symbolization**: Function names may not resolve in flamegraphs for some programs. Symbol resolution is an area of active improvement.
- **Platform**: The eBPF profiler requires Linux (amd64 or arm64) and privileged access to the host.

This feature is suitable for development and testing. Evaluate carefully before production use.

## Architecture

The profiling pipeline has three components:

{{< mermaid >}}
flowchart LR
    A["OTel eBPF Profiler<br/><i>collects CPU profiles<br/>system-wide via eBPF</i>"] -- "OTLP gRPC" --> B["Pyroscope<br/><i>stores & aggregates<br/>profiling data</i>"]
    B -- query --> C["Grafana<br/><i>visualize as<br/>flamegraphs</i>"]
{{< /mermaid >}}

1. **OpenTelemetry eBPF Profiler** runs as an OpenTelemetry Collector distribution with the `profiling` receiver. It attaches eBPF probes to collect CPU stack traces at 97 samples per second from every process on the host.
2. **Pyroscope** receives profiles via OTLP gRPC on port 4040 and stores them.
3. **Grafana** queries Pyroscope to visualize profiles as flamegraphs.

The profiler requires host PID namespace access and several host filesystem mounts (`/proc`, `/sys/kernel`, `/lib/modules`) to resolve stack traces.

## Configure the collector

The profiler runs as a specialized OpenTelemetry Collector. Configure it with a `profiling` receiver and an `otlp_grpc` exporter pointing to Pyroscope:

```yaml
receivers:
  profiling:
    samples_per_second: 97

exporters:
  otlp_grpc:
    endpoint: pyroscope:4040
    tls:
      insecure: true

service:
  pipelines:
    profiles:
      receivers: [profiling]
      exporters: [otlp_grpc]
```

The collector must be started with the `--feature-gates=service.profilesSupport` flag.

### Service name resolution

By default, the profiler sets `process.executable.name` on each profile but does not set `service_name`, which Pyroscope uses as the primary label. To map executable names to service names, configure Pyroscope with an ingestion relabeling rule:

```yaml
limits:
    ingestion_relabeling_rules:
        - action: labelmap
          regex: ^process.executable.name$
          replacement: service_name
```

### Kubernetes metadata enrichment

In Kubernetes, you can add a `k8sattributes` processor to enrich profiles with pod, namespace, deployment, and node metadata:

```yaml
processors:
  k8sattributes/profiles:
    auth_type: serviceAccount
    passthrough: false
    extract:
      metadata:
        - k8s.pod.name
        - k8s.pod.uid
        - k8s.namespace.name
        - k8s.deployment.name
        - k8s.node.name
        - k8s.container.name
        - container.image.name
        - container.image.tag
        - service.name
        - service.namespace
        - service.instance.id
      otel_annotations: true
    pod_association:
      - sources:
          - from: resource_attribute
            name: container.id

service:
  pipelines:
    profiles:
      receivers: [profiling]
      processors: [k8sattributes/profiles]
      exporters: [otlp_grpc]
```

This requires a `ServiceAccount` with RBAC permissions to read pods, namespaces, nodes, and workload resources. See the [example RBAC manifest](https://github.com/grafana/pyroscope/tree/main/examples/grafana-alloy-auto-instrumentation/ebpf-otel/kubernetes/rbac.yaml).

## Deploy with Docker Compose

A minimal Docker Compose setup runs the profiler, Pyroscope, and Grafana:

```yaml
services:
  otel-ebpf-profiler:
    image: otel/opentelemetry-collector-ebpf-profiler:0.147.0
    command:
      - --config=/etc/ebpf-profiler-config.yaml
      - --feature-gates=service.profilesSupport
    privileged: true
    pid: "host"
    volumes:
      - ./config/ebpf-profiler-config.yaml:/etc/ebpf-profiler-config.yaml:ro
      - /sys/kernel/debug:/sys/kernel/debug:ro
      - /sys/fs/cgroup:/sys/fs/cgroup:ro
      - /proc:/proc:ro

  pyroscope:
    image: grafana/pyroscope:1.18.1
    command:
      - -self-profiling.disable-push=true
      - -config.file=/etc/pyroscope.yaml
    ports:
      - "4040:4040"

  grafana:
    image: grafana/grafana:latest
    environment:
      - GF_PLUGINS_PREINSTALL_SYNC=grafana-pyroscope-app
      - GF_AUTH_ANONYMOUS_ENABLED=true
      - GF_AUTH_ANONYMOUS_ORG_ROLE=Admin
      - GF_AUTH_DISABLE_LOGIN_FORM=true
    ports:
      - "3000:3000"
```

For a complete working example, see the [Docker example](https://github.com/grafana/pyroscope/tree/main/examples/grafana-alloy-auto-instrumentation/ebpf-otel/docker).

## Deploy on Kubernetes

On Kubernetes, deploy the profiler as a DaemonSet so it runs on every node:

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: otel-ebpf-profiler
spec:
  selector:
    matchLabels:
      app: otel-ebpf-profiler
  template:
    metadata:
      labels:
        app: otel-ebpf-profiler
    spec:
      hostPID: true
      serviceAccountName: otel-ebpf-profiler
      containers:
        - name: profiler
          image: otel/opentelemetry-collector-ebpf-profiler:0.147.0
          args:
            - "--config=/etc/otel/config.yaml"
            - "--feature-gates=+service.profilesSupport"
          securityContext:
            privileged: true
          volumeMounts:
            - name: sys-kernel
              mountPath: /sys/kernel
              readOnly: true
            - name: proc
              mountPath: /proc
              readOnly: true
      volumes:
        - name: sys-kernel
          hostPath:
            path: /sys/kernel
        - name: proc
          hostPath:
            path: /proc
      tolerations:
        - operator: Exists
```

For a complete working example with kustomize, Pyroscope, Grafana, RBAC, and a sample workload, see the [Kubernetes example](https://github.com/grafana/pyroscope/tree/main/examples/grafana-alloy-auto-instrumentation/ebpf-otel/kubernetes).

Deploy it with:

```bash
git clone --depth 1 --filter=tree:0 --no-checkout https://github.com/grafana/pyroscope.git
cd pyroscope
git sparse-checkout set examples/grafana-alloy-auto-instrumentation/ebpf-otel
git checkout
kubectl apply -k examples/grafana-alloy-auto-instrumentation/ebpf-otel/kubernetes/
```

## Verify

After deploying, open Grafana (http://localhost:3000 for Docker, or port-forward in Kubernetes) and navigate to **Explore** with the Pyroscope data source. You should see profiles grouped by `service_name` appearing within a few minutes.

![Profiles in Grafana](https://github.com/user-attachments/assets/15ff58d4-218a-43dd-9835-df12e13ced3f)
