# pyroscope

![Version: 1.3.2](https://img.shields.io/badge/Version-1.3.2-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 1.2.1](https://img.shields.io/badge/AppVersion-1.2.1-informational?style=flat-square)

🔥 horizontally-scalable, highly-available, multi-tenant continuous profiling aggregation system

## Requirements

| Repository | Name | Version |
|------------|------|---------|
| https://charts.min.io/ | minio(minio) | 4.0.12 |
| https://grafana.github.io/helm-charts | agent(grafana-agent) | 0.25.0 |

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| agent | object | `{"agent":{"clustering":{"enabled":true},"configMap":{"content":"logging {\n  level  = \"info\"\n  format = \"logfmt\"\n}\n\ndiscovery.kubernetes \"pyroscope_kubernetes\" {\n  role = \"pod\"\n}\n\n// The default scrape config allows to define annotations based scraping.\n//\n// For example the following annotations:\n//\n// ```\n// profiles.grafana.com/memory.scrape: \"true\"\n// profiles.grafana.com/memory.port: \"8080\"\n// profiles.grafana.com/cpu.scrape: \"true\"\n// profiles.grafana.com/cpu.port: \"8080\"\n// profiles.grafana.com/goroutine.scrape: \"true\"\n// profiles.grafana.com/goroutine.port: \"8080\"\n// ```\n//\n// will scrape the `memory`, `cpu` and `goroutine` profiles from the `8080` port of the pod.\n//\n// For more information see https://grafana.com/docs/phlare/latest/operators-guide/deploy-kubernetes/#optional-scrape-your-own-workloads-profiles\ndiscovery.relabel \"kubernetes_pods\" {\n  targets = concat(discovery.kubernetes.pyroscope_kubernetes.targets)\n\n  rule {\n    action        = \"drop\"\n    source_labels = [\"__meta_kubernetes_pod_phase\"]\n    regex         = \"Pending|Succeeded|Failed|Completed\"\n  }\n\n  rule {\n    action = \"labelmap\"\n    regex  = \"__meta_kubernetes_pod_label_(.+)\"\n  }\n\n  rule {\n    action        = \"replace\"\n    source_labels = [\"__meta_kubernetes_namespace\"]\n    target_label  = \"namespace\"\n  }\n\n  rule {\n    action        = \"replace\"\n    source_labels = [\"__meta_kubernetes_pod_name\"]\n    target_label  = \"pod\"\n  }\n\n  rule {\n    action        = \"replace\"\n    source_labels = [\"__meta_kubernetes_pod_container_name\"]\n    target_label  = \"container\"\n  }\n}\n{{- $profileTypes := list \"memory\" \"cpu\" \"goroutine\" \"block\" \"mutex\" \"fgprof\" }}\n{{- range $profileTypes }}\n\ndiscovery.relabel \"kubernetes_pods_{{.}}_default_name\" {\n  targets = concat(discovery.relabel.kubernetes_pods.output)\n\n  rule {\n    source_labels = [\"__meta_kubernetes_pod_annotation_profiles_grafana_com_{{.}}_scrape\"]\n    action        = \"keep\"\n    regex         = \"true\"\n  }\n\n  rule {\n    source_labels = [\"__meta_kubernetes_pod_annotation_profiles_grafana_com_{{.}}_port_name\"]\n    action        = \"keep\"\n    regex         = \"\"\n  }\n\n  rule {\n    source_labels = [\"__meta_kubernetes_pod_annotation_profiles_grafana_com_{{.}}_scheme\"]\n    action        = \"replace\"\n    regex         = \"(https?)\"\n    target_label  = \"__scheme__\"\n    replacement   = \"$1\"\n  }\n\n  rule {\n    source_labels = [\"__meta_kubernetes_pod_annotation_profiles_grafana_com_{{.}}_path\"]\n    action        = \"replace\"\n    regex         = \"(.+)\"\n    target_label  = \"__profile_path__\"\n    replacement   = \"$1\"\n  }\n\n  rule {\n    source_labels = [\"__address__\", \"__meta_kubernetes_pod_annotation_profiles_grafana_com_{{.}}_port\"]\n    action        = \"replace\"\n    regex         = \"(.+?)(?::\\\\d+)?;(\\\\d+)\"\n    target_label  = \"__address__\"\n    replacement   = \"$1:$2\"\n  }\n}\n\ndiscovery.relabel \"kubernetes_pods_{{.}}_custom_name\" {\n  targets = concat(discovery.relabel.kubernetes_pods.output)\n\n  rule {\n    source_labels = [\"__meta_kubernetes_pod_annotation_profiles_grafana_com_{{.}}_scrape\"]\n    action        = \"keep\"\n    regex         = \"true\"\n  }\n\n  rule {\n    source_labels = [\"__meta_kubernetes_pod_annotation_profiles_grafana_com_{{.}}_port_name\"]\n    action        = \"drop\"\n    regex         = \"\"\n  }\n\n  rule {\n    source_labels = [\"__meta_kubernetes_pod_container_port_name\"]\n    target_label  = \"__meta_kubernetes_pod_annotation_profiles_grafana_com_{{.}}_port_name\"\n    action        = \"keepequal\"\n  }\n\n  rule {\n    source_labels = [\"__meta_kubernetes_pod_annotation_profiles_grafana_com_{{.}}_scheme\"]\n    action        = \"replace\"\n    regex         = \"(https?)\"\n    target_label  = \"__scheme__\"\n    replacement   = \"$1\"\n  }\n\n  rule {\n    source_labels = [\"__meta_kubernetes_pod_annotation_profiles_grafana_com_{{.}}_path\"]\n    action        = \"replace\"\n    regex         = \"(.+)\"\n    target_label  = \"__profile_path__\"\n    replacement   = \"$1\"\n  }\n\n  rule {\n    source_labels = [\"__address__\", \"__meta_kubernetes_pod_annotation_profiles_grafana_com_{{.}}_port\"]\n    action        = \"replace\"\n    regex         = \"(.+?)(?::\\\\d+)?;(\\\\d+)\"\n    target_label  = \"__address__\"\n    replacement   = \"$1:$2\"\n  }\n}\n\npyroscope.scrape \"pyroscope_scrape_{{.}}\" {\n  clustering {\n    enabled = true\n  }\n\n  targets    = concat(discovery.relabel.kubernetes_pods_{{.}}_default_name.output, discovery.relabel.kubernetes_pods_{{.}}_custom_name.output)\n  forward_to = [pyroscope.write.pyroscope_write.receiver]\n\n  profiling_config {\n    {{- $currentType := . -}}\n    {{- range $profileTypes }}\n    profile.{{if eq . \"cpu\"}}process_cpu{{else}}{{.}}{{end}} {\n      enabled = {{if eq . $currentType}}true{{else}}false{{end}}\n    }\n    {{- if ne . (last $profileTypes) }}{{ printf \"\\n\" }}{{ end }}\n    {{- end }}\n  }\n}\n{{- end }}\n\npyroscope.write \"pyroscope_write\" {\n  endpoint {\n    {{- if hasKey .Values.pyroscope.components \"distributor\" }}\n    url = \"http://{{ include \"pyroscope.fullname\" . }}-distributor.{{ .Release.Namespace }}.svc.cluster.local.:{{ (.Values.pyroscope.components.distributor.service).port | default .Values.pyroscope.service.port}}\"\n    {{- else }}\n    url = \"http://{{ include \"pyroscope.fullname\" . }}.{{ .Release.Namespace }}.svc.cluster.local.:{{ .Values.pyroscope.service.port }}\"\n    {{- end }}\n  }\n}\n","create":true,"name":"grafana-agent-config-pyroscope"}},"controller":{"podAnnotations":{"profiles.grafana.com/cpu.port_name":"http-metrics","profiles.grafana.com/cpu.scrape":"true","profiles.grafana.com/goroutine.port_name":"http-metrics","profiles.grafana.com/goroutine.scrape":"true","profiles.grafana.com/memory.port_name":"http-metrics","profiles.grafana.com/memory.scrape":"true"},"replicas":1,"type":"statefulset"},"enabled":true}` | ----------------------------------- |
| ingress.className | string | `""` |  |
| ingress.enabled | bool | `false` |  |
| minio | object | `{"buckets":[{"name":"grafana-pyroscope-data","policy":"none","purge":false}],"drivesPerNode":2,"enabled":false,"persistence":{"size":"5Gi"},"podAnnotations":{"phlare.grafana.com/port":"9000","phlare.grafana.com/scrape":"true"},"replicas":1,"resources":{"requests":{"cpu":"100m","memory":"128Mi"}},"rootPassword":"supersecret","rootUser":"grafana-pyroscope"}` | ----------------------------------- |
| pyroscope.affinity | object | `{}` |  |
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
