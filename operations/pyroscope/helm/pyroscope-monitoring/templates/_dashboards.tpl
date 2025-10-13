{{/*
Config Map content for dashboard provisioning
*/}}
{{- define "pyroscope-monitoring.dashboards-configmap" -}}
{{/*
List of dashboards
*/}}
{{- $dashboards := list "operational" "v2-metastore" "v2-read-path" "v2-write-path" }}
data:
  grafana-dashboards.yaml: |
    apiVersion: 1
    providers:
      {{- range $dashboard := $dashboards }}
      {{- with $ }}
      - name: "{{ $dashboard }}"
        type: file
        allowUiUpdates: true
        folder: "Pyroscope"
        options:
          path: /otel-lgtm/grafana/conf/provisioning/dashboards/{{$dashboard}}.json
          foldersFromFilesStructure: false
      {{- end }}
      {{- end }}
  {{- range $dashboard := $dashboards }}
  {{- with $ }}
  {{$dashboard}}.json: |
    {{- $content := (include (printf "pyroscope-monitoring.dashboards.%s" $dashboard) . | fromYaml) }}
    {{- if hasKey $content "Error" }}
    {{ fail (get $content "Error") }}
    {{- end }}
    {{- $content | mustToRawJson | nindent  4 }}
  {{- end }}
  {{- end }}
{{- end }}

{{/*
Get hash across all dashboards
*/}}
{{- define "pyroscope-monitoring.dashboards-hash" -}}
{{- include "pyroscope-monitoring.dashboards-configmap" .  | sha256sum }}
{{- end }}

{{/*
Ingest selector: This is the selector to be added to get the most outside metrics available. This depends it the cloud-backend-gataway is deployed or not (only in Grafana Cloud)
*/}}
{{- define "pyroscope-monitoring.dashboards-ingest-selector" -}}
{{- if .Values.dashboards.cloudBackendGateway }}{{ .Values.dashboards.cloudBackendGatewaySelector }}{{ else }}{{ .Values.dashboards.ingestSelector }}{{ end -}}
{{- end }}
