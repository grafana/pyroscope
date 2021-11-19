# Pyroscope pull mode in Kubernetes

This example demonstrates how Pyroscope can be used to scrape pprof profiles from remote targets.

### 1. Run minikube kubernetes cluster locally (optional)

In this example we use minikube for sake of simplicity, visit [minikube documentation](https://minikube.sigs.k8s.io/docs/start/)
to learn more.

```shell
minikube start
```

### 2. Install Pyroscope with Helm chart

The official [Pyroscope Helm chart](https://github.com/pyroscope-io/helm-chart) deploys Pyroscope server and creates proper RBAC roles:

```shell
helm repo add pyroscope-io https://pyroscope-io.github.io/helm-chart
helm install demo pyroscope-io/pyroscope -f values.yaml
```

Note that we apply configuration defined in `values.yaml`: Pyroscope uses exactly the same discovery mechanisms as
Prometheus does in order to ensure smooth user experience, and it fully supports
[Kubernetes Service Discovery](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#kubernetes_sd_config):

```yaml
---
pyroscopeConfigs:
  log-level: debug
  scrape-configs:
  - job-name: 'kubernetes-pods'
    enabled-profiles: [ cpu, mem ]
    kubernetes-sd-configs:
      - role: pod
    relabel-configs:
      - source-labels: [__meta_kubernetes_pod_annotation_pyroscope_io_scrape]
        action: keep
        regex: true
      - source-labels:
          [__meta_kubernetes_pod_annotation_pyroscope_io_application_name]
        action: replace
        target-label: __name__
      - source-labels: [__meta_kubernetes_pod_annotation_pyroscope_io_scheme]
        action: replace
        regex: (https?)
        target-label: __scheme__
      - source-labels:
          [__address__, __meta_kubernetes_pod_annotation_pyroscope_io_port]
        action: replace
        regex: ([^:]+)(?::\d+)?;(\d+)
        replacement: $1:$2
        target-label: __address__
      - action: labelmap
        regex: __meta_kubernetes_pod_label_(.+)
      - source-labels: [__meta_kubernetes_namespace]
        action: replace
        target-label: kubernetes_namespace
      - source-labels: [__meta_kubernetes_pod_name]
        action: replace
        target-label: kubernetes_pod_name
      - source-labels: [__meta_kubernetes_pod_phase]
        regex: Pending|Succeeded|Failed|Completed
        action: drop
      - action: labelmap
        regex: __meta_kubernetes_pod_annotation_pyroscope_io_profile_(.+)
        replacement: __profile_$1
```

### 3. Deploy Hot R.O.D. application

As a sample application we use slightly modified Jaeger [Hot R.O.D.](https://github.com/jaegertracing/jaeger/tree/master/examples/hotrod) demo – 
the only difference is that we enabled built-in Go `pprof` HTTP endpoints. You can find the modified code in the [hotrod-goland](https://github.com/pyroscope-io/hotrod-golang) repository.

Kubernetes resources are defined in [`manifests.yaml`](manifests.yaml): notice pod labels defined – by this we instruct Pyroscope to 
scrape cpu and memory profiles at `:6060`:
```yaml
pyroscope.io/scrape: "true"
pyroscope.io/application-name: "hotrod"
pyroscope.io/profile-cpu-enabled: "true"
pyroscope.io/profile-mem-enabled: "true"
pyroscope.io/port: "6060"
```

```shell
kubectl apply -f manifests.yaml
```

### 4. Open Pyroscope UI

Now that everything is set up, you can browse profiling data via Pyroscope UI:
```shell
minikube service demo-pyroscope
```
