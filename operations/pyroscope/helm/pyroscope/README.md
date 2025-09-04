# pyroscope

![Version: 1.15.0](https://img.shields.io/badge/Version-1.15.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 1.14.1](https://img.shields.io/badge/AppVersion-1.14.1-informational?style=flat-square)

🔥 horizontally-scalable, highly-available, multi-tenant continuous profiling aggregation system

**Homepage:** <https://grafana.com/oss/pyroscope/>

## Source Code

* <https://github.com/grafana/pyroscope>
* <https://github.com/grafana/pyroscope/tree/main/operations/pyroscope/helm/pyroscope>

## Requirements

| Repository | Name | Version |
|------------|------|---------|
| https://charts.min.io/ | minio(minio) | 4.0.12 |
| https://grafana.github.io/helm-charts | alloy(alloy) | 1.0.1 |
| https://grafana.github.io/helm-charts | agent(grafana-agent) | 0.25.0 |

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| agent | object | `{"agent":{"clustering":{"enabled":true},"configMap":{"create":false,"name":"grafana-agent-config-pyroscope"}},"controller":{"podAnnotations":{"profiles.grafana.com/cpu.port_name":"http-metrics","profiles.grafana.com/cpu.scrape":"true","profiles.grafana.com/goroutine.port_name":"http-metrics","profiles.grafana.com/goroutine.scrape":"true","profiles.grafana.com/memory.port_name":"http-metrics","profiles.grafana.com/memory.scrape":"true"},"replicas":1,"type":"statefulset"},"enabled":false}` | ----------------------------------- |
| alloy | object | `{"alloy":{"clustering":{"enabled":true},"configMap":{"create":false,"name":"alloy-config-pyroscope"},"stabilityLevel":"public-preview"},"controller":{"podAnnotations":{"profiles.grafana.com/cpu.port_name":"http-metrics","profiles.grafana.com/cpu.scrape":"true","profiles.grafana.com/goroutine.port_name":"http-metrics","profiles.grafana.com/goroutine.scrape":"true","profiles.grafana.com/memory.port_name":"http-metrics","profiles.grafana.com/memory.scrape":"true","profiles.grafana.com/service_git_ref":"v1.8.1","profiles.grafana.com/service_repository":"https://github.com/grafana/alloy"},"replicas":1,"type":"statefulset"},"enabled":true}` | ----------------------------------- |
| architecture.microservices.enabled | bool | `false` |  |
| architecture.microservices.v1.ad-hoc-profiles.kind | string | `"Deployment"` |  |
| architecture.microservices.v1.ad-hoc-profiles.replicaCount | int | `1` |  |
| architecture.microservices.v1.ad-hoc-profiles.resources.limits.memory | string | `"4Gi"` |  |
| architecture.microservices.v1.ad-hoc-profiles.resources.requests.cpu | float | `0.1` |  |
| architecture.microservices.v1.ad-hoc-profiles.resources.requests.memory | string | `"16Mi"` |  |
| architecture.microservices.v1.compactor.kind | string | `"StatefulSet"` |  |
| architecture.microservices.v1.compactor.persistence.enabled | bool | `false` |  |
| architecture.microservices.v1.compactor.replicaCount | int | `3` |  |
| architecture.microservices.v1.compactor.resources.limits.memory | string | `"16Gi"` |  |
| architecture.microservices.v1.compactor.resources.requests.cpu | int | `1` |  |
| architecture.microservices.v1.compactor.resources.requests.memory | string | `"8Gi"` |  |
| architecture.microservices.v1.compactor.terminationGracePeriodSeconds | int | `1200` |  |
| architecture.microservices.v1.distributor.kind | string | `"Deployment"` |  |
| architecture.microservices.v1.distributor.replicaCount | int | `2` |  |
| architecture.microservices.v1.distributor.resources.limits.memory | string | `"1Gi"` |  |
| architecture.microservices.v1.distributor.resources.requests.cpu | string | `"500m"` |  |
| architecture.microservices.v1.distributor.resources.requests.memory | string | `"256Mi"` |  |
| architecture.microservices.v1.ingester.kind | string | `"StatefulSet"` |  |
| architecture.microservices.v1.ingester.replicaCount | int | `3` |  |
| architecture.microservices.v1.ingester.resources.limits.memory | string | `"16Gi"` |  |
| architecture.microservices.v1.ingester.resources.requests.cpu | int | `1` |  |
| architecture.microservices.v1.ingester.resources.requests.memory | string | `"8Gi"` |  |
| architecture.microservices.v1.ingester.terminationGracePeriodSeconds | int | `600` |  |
| architecture.microservices.v1.querier.kind | string | `"Deployment"` |  |
| architecture.microservices.v1.querier.replicaCount | int | `3` |  |
| architecture.microservices.v1.querier.resources.limits.memory | string | `"1Gi"` |  |
| architecture.microservices.v1.querier.resources.requests.cpu | int | `1` |  |
| architecture.microservices.v1.querier.resources.requests.memory | string | `"256Mi"` |  |
| architecture.microservices.v1.query-frontend.kind | string | `"Deployment"` |  |
| architecture.microservices.v1.query-frontend.replicaCount | int | `2` |  |
| architecture.microservices.v1.query-frontend.resources.limits.memory | string | `"1Gi"` |  |
| architecture.microservices.v1.query-frontend.resources.requests.cpu | string | `"100m"` |  |
| architecture.microservices.v1.query-frontend.resources.requests.memory | string | `"256Mi"` |  |
| architecture.microservices.v1.query-scheduler.kind | string | `"Deployment"` |  |
| architecture.microservices.v1.query-scheduler.replicaCount | int | `2` |  |
| architecture.microservices.v1.query-scheduler.resources.limits.memory | string | `"1Gi"` |  |
| architecture.microservices.v1.query-scheduler.resources.requests.cpu | string | `"100m"` |  |
| architecture.microservices.v1.query-scheduler.resources.requests.memory | string | `"256Mi"` |  |
| architecture.microservices.v1.store-gateway.kind | string | `"StatefulSet"` |  |
| architecture.microservices.v1.store-gateway.persistence.enabled | bool | `false` |  |
| architecture.microservices.v1.store-gateway.readinessProbe.initialDelaySeconds | int | `60` |  |
| architecture.microservices.v1.store-gateway.replicaCount | int | `3` |  |
| architecture.microservices.v1.store-gateway.resources.limits.memory | string | `"16Gi"` |  |
| architecture.microservices.v1.store-gateway.resources.requests.cpu | int | `1` |  |
| architecture.microservices.v1.store-gateway.resources.requests.memory | string | `"8Gi"` |  |
| architecture.microservices.v1.tenant-settings.kind | string | `"Deployment"` |  |
| architecture.microservices.v1.tenant-settings.replicaCount | int | `1` |  |
| architecture.microservices.v1.tenant-settings.resources.limits.memory | string | `"4Gi"` |  |
| architecture.microservices.v1.tenant-settings.resources.requests.cpu | float | `0.1` |  |
| architecture.microservices.v1.tenant-settings.resources.requests.memory | string | `"16Mi"` |  |
| architecture.microservices.v2.ad-hoc-profiles.kind | string | `"Deployment"` |  |
| architecture.microservices.v2.ad-hoc-profiles.replicaCount | int | `1` |  |
| architecture.microservices.v2.ad-hoc-profiles.resources.limits.memory | string | `"4Gi"` |  |
| architecture.microservices.v2.ad-hoc-profiles.resources.requests.cpu | float | `0.1` |  |
| architecture.microservices.v2.ad-hoc-profiles.resources.requests.memory | string | `"16Mi"` |  |
| architecture.microservices.v2.compaction-worker.kind | string | `"StatefulSet"` |  |
| architecture.microservices.v2.compaction-worker.persistence.enabled | bool | `false` |  |
| architecture.microservices.v2.compaction-worker.replicaCount | int | `3` |  |
| architecture.microservices.v2.compaction-worker.resources.limits.memory | string | `"16Gi"` |  |
| architecture.microservices.v2.compaction-worker.resources.requests.cpu | int | `1` |  |
| architecture.microservices.v2.compaction-worker.resources.requests.memory | string | `"8Gi"` |  |
| architecture.microservices.v2.compaction-worker.terminationGracePeriodSeconds | int | `1200` |  |
| architecture.microservices.v2.distributor.kind | string | `"Deployment"` |  |
| architecture.microservices.v2.distributor.replicaCount | int | `2` |  |
| architecture.microservices.v2.distributor.resources.limits.memory | string | `"1Gi"` |  |
| architecture.microservices.v2.distributor.resources.requests.cpu | string | `"500m"` |  |
| architecture.microservices.v2.distributor.resources.requests.memory | string | `"256Mi"` |  |
| architecture.microservices.v2.metastore.kind | string | `"StatefulSet"` |  |
| architecture.microservices.v2.metastore.persistence.enabled | bool | `false` |  |
| architecture.microservices.v2.metastore.replicaCount | int | `3` |  |
| architecture.microservices.v2.metastore.resources.limits.memory | string | `"16Gi"` |  |
| architecture.microservices.v2.metastore.resources.requests.cpu | int | `1` |  |
| architecture.microservices.v2.metastore.resources.requests.memory | string | `"8Gi"` |  |
| architecture.microservices.v2.metastore.terminationGracePeriodSeconds | int | `1200` |  |
| architecture.microservices.v2.query-backend.kind | string | `"Deployment"` |  |
| architecture.microservices.v2.query-backend.replicaCount | int | `3` |  |
| architecture.microservices.v2.query-backend.resources.limits.memory | string | `"1Gi"` |  |
| architecture.microservices.v2.query-backend.resources.requests.cpu | int | `1` |  |
| architecture.microservices.v2.query-backend.resources.requests.memory | string | `"256Mi"` |  |
| architecture.microservices.v2.query-frontend.kind | string | `"Deployment"` |  |
| architecture.microservices.v2.query-frontend.replicaCount | int | `2` |  |
| architecture.microservices.v2.query-frontend.resources.limits.memory | string | `"1Gi"` |  |
| architecture.microservices.v2.query-frontend.resources.requests.cpu | string | `"100m"` |  |
| architecture.microservices.v2.query-frontend.resources.requests.memory | string | `"256Mi"` |  |
| architecture.microservices.v2.segment-writer.kind | string | `"StatefulSet"` |  |
| architecture.microservices.v2.segment-writer.replicaCount | int | `3` |  |
| architecture.microservices.v2.segment-writer.resources.limits.memory | string | `"16Gi"` |  |
| architecture.microservices.v2.segment-writer.resources.requests.cpu | int | `1` |  |
| architecture.microservices.v2.segment-writer.resources.requests.memory | string | `"8Gi"` |  |
| architecture.microservices.v2.segment-writer.terminationGracePeriodSeconds | int | `600` |  |
| architecture.microservices.v2.tenant-settings.kind | string | `"Deployment"` |  |
| architecture.microservices.v2.tenant-settings.replicaCount | int | `1` |  |
| architecture.microservices.v2.tenant-settings.resources.limits.memory | string | `"4Gi"` |  |
| architecture.microservices.v2.tenant-settings.resources.requests.cpu | float | `0.1` |  |
| architecture.microservices.v2.tenant-settings.resources.requests.memory | string | `"16Mi"` |  |
| architecture.storageLayer.v1 | bool | `true` |  |
| architecture.storageLayer.v2 | bool | `false` |  |
| ingress.annotations | object | `{}` |  |
| ingress.className | string | `""` |  |
| ingress.enabled | bool | `false` |  |
| ingress.labels | object | `{}` |  |
| ingress.pathType | string | `"ImplementationSpecific"` |  |
| minio | object | `{"buckets":[{"name":"grafana-pyroscope-data","policy":"none","purge":false}],"drivesPerNode":2,"enabled":false,"persistence":{"size":"5Gi"},"podAnnotations":{},"replicas":1,"resources":{"requests":{"cpu":"100m","memory":"128Mi"}},"rootPassword":"supersecret","rootUser":"grafana-pyroscope"}` | ----------------------------------- |
| minio.enabled | bool | `true` |  |
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
| pyroscope.image.pullPolicy | string | `"IfNotPresent"` |  |
| pyroscope.image.repository | string | `"grafana/pyroscope"` |  |
| pyroscope.image.tag | string | `""` |  |
| pyroscope.imagePullSecrets | list | `[]` |  |
| pyroscope.initContainers | list | `[]` |  |
| pyroscope.memberlist.port | int | `7946` |  |
| pyroscope.memberlist.port_name | string | `"memberlist"` |  |
| pyroscope.nameOverride | string | `""` |  |
| pyroscope.nodeSelector | object | `{}` |  |
| pyroscope.persistence.accessModes[0] | string | `"ReadWriteOnce"` |  |
| pyroscope.persistence.annotations | object | `{}` |  |
| pyroscope.persistence.enabled | bool | `false` |  |
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

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.8.1](https://github.com/norwoodj/helm-docs/releases/v1.8.1)
