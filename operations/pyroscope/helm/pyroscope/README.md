# pyroscope

![Version: 1.3.4](https://img.shields.io/badge/Version-1.3.4-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 1.2.1](https://img.shields.io/badge/AppVersion-1.2.1-informational?style=flat-square)

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
| pyroscope.image.tag | string | `"1.2.1"` |  |
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

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.8.1](https://github.com/norwoodj/helm-docs/releases/v1.8.1)
