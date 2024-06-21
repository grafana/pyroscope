# pyroscope

![Version: 1.6.1](https://img.shields.io/badge/Version-1.6.1-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 1.6.1](https://img.shields.io/badge/AppVersion-1.6.1-informational?style=flat-square)

🔥 horizontally-scalable, highly-available, multi-tenant continuous profiling aggregation system

## Requirements

| Repository | Name | Version |
|------------|------|---------|
| https://charts.min.io/ | minio(minio) | 4.0.12 |
| https://grafana.github.io/helm-charts | agent(grafana-agent) | 0.25.0 |

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| agent | object | `{"agent":{"clustering":{"enabled":true},"configMap":{"create":false,"name":"grafana-agent-config-pyroscope"}},"controller":{"podAnnotations":{"profiles.grafana.com/cpu.port_name":"http-metrics","profiles.grafana.com/cpu.scrape":"true","profiles.grafana.com/goroutine.port_name":"http-metrics","profiles.grafana.com/goroutine.scrape":"true","profiles.grafana.com/memory.port_name":"http-metrics","profiles.grafana.com/memory.scrape":"true"},"replicas":1,"type":"statefulset"},"enabled":true}` | ----------------------------------- |
| ingress.className | string | `""` |  |
| ingress.enabled | bool | `false` |  |
| minio | object | `{"buckets":[{"name":"grafana-pyroscope-data","policy":"none","purge":false}],"drivesPerNode":2,"enabled":false,"persistence":{"size":"5Gi"},"podAnnotations":{"phlare.grafana.com/port":"9000","phlare.grafana.com/scrape":"true"},"replicas":1,"resources":{"requests":{"cpu":"100m","memory":"128Mi"}},"rootPassword":"supersecret","rootUser":"grafana-pyroscope"}` | ----------------------------------- |
| pyroscope.affinity | object | `{}` |  |
| pyroscope.cluster_domain | string | `".cluster.local."` | Kubernetes cluster domain suffix for DNS discovery |
| pyroscope.components | object | `{}` |  |
| pyroscope.config | string | The config depends on other values been set, details can be found in [`values.yaml`](./values.yaml) | Contains Phlare's configuration as a string. |
| pyroscope.dnsPolicy | string | `"ClusterFirst"` |  |
| pyroscope.extraArgs."log.level" | string | `"debug"` |  |
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
| pyroscope.rbac.create | bool | `true` |  |
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
| pyroscope.structuredConfig | object | `{}` | Allows to override Phlare's configuration using structured format. |
| pyroscope.tenantOverrides | object | `{}` | Allows to add tenant specific overrides to the default limit configuration. |
| pyroscope.tolerations | list | `[]` |  |
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
Autogenerated from chart metadata using [helm-docs v1.13.1](https://github.com/norwoodj/helm-docs/releases/v1.13.1)
