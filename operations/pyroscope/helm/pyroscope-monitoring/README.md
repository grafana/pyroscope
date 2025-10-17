# pyroscope-monitoring

![Version: 0.1.0](https://img.shields.io/badge/Version-0.1.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.0.0](https://img.shields.io/badge/AppVersion-0.0.0-informational?style=flat-square)

A Helm chart for monitoring Grafana Pyroscope

## Requirements

| Repository | Name | Version |
|------------|------|---------|
| https://grafana.github.io/helm-charts | monitoring(k8s-monitoring) | 3.5.3 |

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` |  |
| dashboards.cadvisorSelector | string | `"job=~\"(.*/)?cadvisor\""` |  |
| dashboards.cloudBackendGateway | bool | `false` |  |
| dashboards.cloudBackendGatewaySelector | string | `"container=~\"cortex-gw(-internal)?\""` |  |
| dashboards.cluster | string | `"pyroscope-dev"` |  |
| dashboards.ingestSelector | string | `"container=~\"pyroscope|distributor|query-frontend\""` |  |
| dashboards.kubeStateMetricsSelector | string | `"job=~\"(.*/)?kube-state-metrics\""` |  |
| dashboards.namespace | string | `"default"` |  |
| dashboards.namespaceRegex | string | `".*"` |  |
| dashboards.tenantQuery | string | `"sum by (tenant, slug, org_name, environment) (\n  rate(pyroscope_distributor_received_decompressed_bytes_sum{cluster=~\"$cluster\",namespace=~\"$namespace\"}[$__rate_interval])\n)\n"` |  |
| fullnameOverride | string | `""` |  |
| image.pullPolicy | string | `"IfNotPresent"` |  |
| image.repository | string | `"grafana/otel-lgtm"` |  |
| image.tag | string | `"0.11.10"` |  |
| imagePullSecrets | list | `[]` |  |
| nameOverride | string | `""` |  |
| nodeSelector | object | `{}` |  |
| podAnnotations | object | `{}` |  |
| podLabels | object | `{}` |  |
| podSecurityContext | object | `{}` |  |
| replicaCount | int | `1` |  |
| resources | object | `{}` |  |
| securityContext | object | `{}` |  |
| service.deployStaticName | bool | `true` |  |
| service.type | string | `"ClusterIP"` |  |
| tolerations | list | `[]` |  |

