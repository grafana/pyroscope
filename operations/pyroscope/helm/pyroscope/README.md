# pyroscope

![Version: 1.16.0](https://img.shields.io/badge/Version-1.16.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 1.16.0](https://img.shields.io/badge/AppVersion-1.16.0-informational?style=flat-square)

🔥 horizontally-scalable, highly-available, multi-tenant continuous profiling aggregation system

**Homepage:** <https://grafana.com/oss/pyroscope/>

## Source Code

* <https://github.com/grafana/pyroscope>
* <https://github.com/grafana/pyroscope/tree/main/operations/pyroscope/helm/pyroscope>

## Requirements

| Repository | Name | Version |
|------------|------|---------|
| https://charts.min.io/ | minio(minio) | 4.1.0 |
| https://grafana.github.io/helm-charts | alloy(alloy) | 1.4.0 |
| https://grafana.github.io/helm-charts | agent(grafana-agent) | 0.44.2 |

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| agent | object | `{"agent":{"clustering":{"enabled":true},"configMap":{"create":false,"name":"grafana-agent-config-pyroscope"}},"controller":{"podAnnotations":{"profiles.grafana.com/cpu.port_name":"http-metrics","profiles.grafana.com/cpu.scrape":"true","profiles.grafana.com/goroutine.port_name":"http-metrics","profiles.grafana.com/goroutine.scrape":"true","profiles.grafana.com/memory.port_name":"http-metrics","profiles.grafana.com/memory.scrape":"true"},"replicas":1,"type":"statefulset"},"enabled":false}` | ----------------------------------- |
| alloy | object | `{"alloy":{"clustering":{"enabled":true},"configMap":{"create":false,"name":"alloy-config-pyroscope"},"stabilityLevel":"public-preview"},"controller":{"podAnnotations":{"profiles.grafana.com/cpu.port_name":"http-metrics","profiles.grafana.com/cpu.scrape":"true","profiles.grafana.com/goroutine.port_name":"http-metrics","profiles.grafana.com/goroutine.scrape":"true","profiles.grafana.com/memory.port_name":"http-metrics","profiles.grafana.com/memory.scrape":"true","profiles.grafana.com/service_git_ref":"v1.8.1","profiles.grafana.com/service_repository":"https://github.com/grafana/alloy"},"replicas":1,"type":"statefulset"},"enabled":true}` | ----------------------------------- |
| architecture.deployUnifiedServices | bool | `false` | Deploy unified write/read services. These endpoints will can be used no matter if the helm chart is configured as single-binary or microservices |
| architecture.microservices.clusterLabelSuffix | string | `"-micro-services"` | Memberlist cluster label that will be used for all members of this cluster |
| architecture.microservices.enabled | bool | `false` | Enable micro-services deployment mode. This is recommend for larger scale deployment and allow right size each aspect of Pyroscope. |
| architecture.overwriteResources | object | `{}` | This flag is useful for testing, it will overwrite all pods resource statements with its contents |
| architecture.storage.migration.ingesterWeight | float | `1` | Specifies the fraction [0:1] that should be send to the v1 write path / ingester in combined mode. 0 means no traffics is sent to ingester. 1 means 100% of requests are sent to ingester. |
| architecture.storage.migration.queryBackend | bool | `true` | Specify a time stamp from when the v2 read path should serve traffic. |
| architecture.storage.migration.queryBackendFrom | string | `"auto"` | Specify a time stamp from when the v2 read path should serve traffic. |
| architecture.storage.migration.segmentWriterWeight | float | `1` | Specifies the fraction [0:1] that should be send to the v2 write path / segment-writer in combined mode. 0 means no traffics is sent to segment-writer. 1 means 100% of requests are sent to segment-writer. |
| architecture.storage.v1 | bool | `true` | Enable v1 storage layer. |
| architecture.storage.v2 | bool | `false` | Enable v2 storage layer. |
| ingress.annotations | object | `{}` |  |
| ingress.className | string | `""` |  |
| ingress.enabled | bool | `false` |  |
| ingress.labels | object | `{}` |  |
| ingress.pathType | string | `"ImplementationSpecific"` |  |
| minio | object | `{"buckets":[{"name":"grafana-pyroscope-data","policy":"none","purge":false}],"drivesPerNode":2,"enabled":false,"persistence":{"size":"5Gi"},"podAnnotations":{},"replicas":1,"resources":{"requests":{"cpu":"100m","memory":"128Mi"}},"rootPassword":"supersecret","rootUser":"grafana-pyroscope"}` | ----------------------------------- |
| pyroscope.affinity | object | `{}` |  |
| pyroscope.cluster_domain | string | `".cluster.local."` | Kubernetes cluster domain suffix for DNS discovery |
| pyroscope.components | object | `{}` |  |
| pyroscope.config | string | The config depends on other values been set, details can be found in [`values.yaml`](./values.yaml) | Contains Pyroscope's configuration as a string. |
| pyroscope.disableSelfProfile | bool | `true` | Enable or disable Self profile push, useful to test |
| pyroscope.dnsPolicy | string | `"ClusterFirst"` |  |
| pyroscope.extraArgs."log.level" | string | `"debug"` |  |
| pyroscope.extraCustomEnvVars | object | `{}` |  |
| pyroscope.extraEnvFrom | list | `[]` | Environment variables from secrets or configmaps to add to the pods |
| pyroscope.extraEnvVars | object | `{}` |  |
| pyroscope.extraLabels | object | `{}` |  |
| pyroscope.extraVolumeMounts | list | `[]` |  |
| pyroscope.extraVolumes | list | `[]` |  |
| pyroscope.fullnameOverride | string | `""` |  |
| pyroscope.grpc.port | int | `9095` |  |
| pyroscope.grpc.port_name | string | `"grpc"` |  |
| pyroscope.image.pullPolicy | string | `"IfNotPresent"` |  |
| pyroscope.image.repository | string | `"grafana/pyroscope"` |  |
| pyroscope.image.tag | string | `""` |  |
| pyroscope.imagePullSecrets | list | `[]` |  |
| pyroscope.initContainers | list | `[]` |  |
| pyroscope.memberlist.port | int | `7946` |  |
| pyroscope.memberlist.port_name | string | `"memberlist"` |  |
| pyroscope.metastore.port | int | `9099` |  |
| pyroscope.metastore.port_name | string | `"raft"` |  |
| pyroscope.nameOverride | string | `""` |  |
| pyroscope.nodeSelector | object | `{}` |  |
| pyroscope.persistence.accessModes[0] | string | `"ReadWriteOnce"` |  |
| pyroscope.persistence.annotations | object | `{}` |  |
| pyroscope.persistence.enabled | bool | `false` |  |
| pyroscope.persistence.metastore.subPath | string | `".metastore"` |  |
| pyroscope.persistence.shared.subPath | string | `".shared"` |  |
| pyroscope.persistence.size | string | `"10Gi"` |  |
| pyroscope.podAnnotations."profiles.grafana.com/cpu.port_name" | string | `"http2"` |  |
| pyroscope.podAnnotations."profiles.grafana.com/cpu.scrape" | string | `"true"` |  |
| pyroscope.podAnnotations."profiles.grafana.com/goroutine.port_name" | string | `"http2"` |  |
| pyroscope.podAnnotations."profiles.grafana.com/goroutine.scrape" | string | `"true"` |  |
| pyroscope.podAnnotations."profiles.grafana.com/memory.port_name" | string | `"http2"` |  |
| pyroscope.podAnnotations."profiles.grafana.com/memory.scrape" | string | `"true"` |  |
| pyroscope.podDisruptionBudget.enabled | bool | `true` |  |
| pyroscope.podDisruptionBudget.maxUnavailable | int | `1` |  |
| pyroscope.podSecurityContext.fsGroup | int | `10001` |  |
| pyroscope.podSecurityContext.runAsNonRoot | bool | `true` |  |
| pyroscope.podSecurityContext.runAsUser | int | `10001` |  |
| pyroscope.replicaCount | int | `1` |  |
| pyroscope.resources | object | `{}` |  |
| pyroscope.securityContext | object | `{}` |  |
| pyroscope.service.annotations | object | `{}` |  |
| pyroscope.service.headlessAnnotations | object | `{}` |  |
| pyroscope.service.port | int | `4040` |  |
| pyroscope.service.port_name | string | `"http2"` |  |
| pyroscope.service.scheme | string | `"HTTP"` |  |
| pyroscope.service.type | string | `"ClusterIP"` |  |
| pyroscope.serviceAccount.annotations | object | `{}` |  |
| pyroscope.serviceAccount.create | bool | `true` |  |
| pyroscope.serviceAccount.name | string | `""` |  |
| pyroscope.structuredConfig | object | `{}` | Allows to override Pyroscope's configuration using structured format. |
| pyroscope.tenantOverrides | object | `{}` | Allows to add tenant specific overrides to the default limit configuration. |
| pyroscope.tolerations | list | `[]` |  |
| pyroscope.topologySpreadConstraints | list | `[]` | Topology Spread Constraints |
| serviceMonitor.annotations | object | `{}` | ServiceMonitor annotations |
| serviceMonitor.enabled | bool | `false` | If enabled, ServiceMonitor resources for Prometheus Operator are created |
| serviceMonitor.interval | string | `nil` | ServiceMonitor scrape interval |
| serviceMonitor.labels | object | `{}` | Additional ServiceMonitor labels |
| serviceMonitor.matchExpressions | list | `[]` | Optional expressions to match on |
| serviceMonitor.metricRelabelings | list | `[]` | ServiceMonitor metric relabel configs to apply to samples before ingestion https://github.com/prometheus-operator/prometheus-operator/blob/main/Documentation/api.md#endpoint |
| serviceMonitor.namespaceSelector | object | `{}` | Namespace selector for ServiceMonitor resources |
| serviceMonitor.relabelings | list | `[]` | ServiceMonitor relabel configs to apply to samples before scraping https://github.com/prometheus-operator/prometheus-operator/blob/master/Documentation/api.md#relabelconfig |
| serviceMonitor.scheme | string | `"http"` | ServiceMonitor will use http by default, but you can pick https as well |
| serviceMonitor.scrapeTimeout | string | `nil` | ServiceMonitor scrape timeout in Go duration format (e.g. 15s) |
| serviceMonitor.targetLabels | list | `[]` | ServiceMonitor will add labels from the service to the Prometheus metric https://github.com/prometheus-operator/prometheus-operator/blob/main/Documentation/api.md#servicemonitorspec |
| serviceMonitor.tlsConfig | string | `nil` | ServiceMonitor will use these tlsConfig settings to make the health check requests |

