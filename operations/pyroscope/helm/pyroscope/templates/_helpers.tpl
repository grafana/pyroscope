{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "pyroscope.name" -}}
{{- default .Chart.Name .Values.pyroscope.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "pyroscope.fullname" -}}
{{- if .Values.pyroscope.fullnameOverride }}
{{- .Values.pyroscope.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.pyroscope.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "pyroscope.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "pyroscope.labels" -}}
helm.sh/chart: {{ include "pyroscope.chart" . }}
{{ include "pyroscope.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- range $k, $v := .Values.pyroscope.extraLabels }}
{{$k}}: {{ $v | quote }}
{{- end }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "pyroscope.selectorLabels" -}}
app.kubernetes.io/name: {{ include "pyroscope.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Pod template labels. extraLabels is pyroscope.extraLabels merged with the
component's own extraLabels, so a component can add or override labels.
*/}}
{{- define "pyroscope.templateLabels" -}}
{{ include "pyroscope.selectorLabels" .ctx }}
{{- range $k, $v := .extraLabels }}
{{$k}}: {{ $v | quote }}
{{- end }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "pyroscope.serviceAccountName" -}}
{{- if .Values.pyroscope.serviceAccount.create }}
{{- default (include "pyroscope.fullname" .) .Values.pyroscope.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.pyroscope.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create a list of components that should be deployed.
*/}}
{{- define "pyroscope.components" -}}
{{- $full_name := (include "pyroscope.fullname" .) }}
{{- $components := dict }}
{{- if .Values.architecture.microservices.enabled }}
{{-   if .Values.architecture.storage.v1 }}
{{-     $components = mustMergeOverwrite (deepCopy $components) (.Values.architecture.microservices.v1 | default dict)}}
{{-   end }}
{{-   if .Values.architecture.storage.v2 }}
{{-     $components = mustMergeOverwrite (deepCopy $components) (.Values.architecture.microservices.v2 | default dict)}}
{{-   end }}
{{- end }}
{{- if .Values.architecture.microservices.enabled }}
{{-   range $k, $v := (.Values.pyroscope.components | default dict) }}
{{-     if hasKey $components $k }}
{{-       $_ := set $components $k (mustMergeOverwrite (deepCopy (index $components $k)) $v) }}
{{-     end }}
{{-   end }}
{{- else }}
{{-   $components = mustMergeOverwrite (deepCopy $components) (.Values.pyroscope.components | default dict)}}
{{- end }}
{{- range $k, $v := $components }}
{{- $v :=  set $v "name" (printf "%s-%s" $full_name $k) }}
{{$k}}: {{ $v | toJson }}
{{- end }}
{{/*
If no components are defined fall back to single binary statefulset
*/}}
{{- if eq (len $components) 0 }}
all: {kind: "StatefulSet", name: "{{$full_name}}"}
{{- end }}
{{- end }}


{{/*
Return the URL for sending profiles to Pyroscope.
*/}}
{{- define "pyroscope.write_url" -}}
{{- $components := (fromYaml (include "pyroscope.components" .)) -}}
{{- if .Values.architecture.deployUnifiedServices -}}
http://{{ include "pyroscope.fullname" . }}-write.{{ .Release.Namespace }}.svc{{ .Values.pyroscope.cluster_domain }}
{{- else if hasKey $components "distributor" -}}
http://{{ include "pyroscope.fullname" . }}-distributor.{{ .Release.Namespace }}.svc{{ .Values.pyroscope.cluster_domain }}:{{ ($components.distributor.service).port | default .Values.pyroscope.service.port}}
{{- else -}}
http://{{ include "pyroscope.fullname" . }}.{{ .Release.Namespace }}.svc{{ .Values.pyroscope.cluster_domain }}:{{ .Values.pyroscope.service.port }}
{{- end }}
{{- end }}

{{/*
Return the URL for querying profiles from Pyroscope.
*/}}
{{- define "pyroscope.read_url" -}}
{{- $components := (fromYaml (include "pyroscope.components" .)) -}}
{{- if .Values.architecture.deployUnifiedServices -}}
http://{{ include "pyroscope.fullname" . }}-read.{{ .Release.Namespace }}.svc{{ .Values.pyroscope.cluster_domain }}
{{- else if hasKey $components "query-frontend" -}}
{{- $port := ((index $components "query-frontend").service).port | default .Values.pyroscope.service.port }}
http://{{ include "pyroscope.fullname" . }}-query-frontend.{{ .Release.Namespace }}.svc{{ .Values.pyroscope.cluster_domain }}:{{ $port }}
{{- else -}}
http://{{ include "pyroscope.fullname" . }}.{{ .Release.Namespace }}.svc{{ .Values.pyroscope.cluster_domain }}:{{ .Values.pyroscope.service.port }}
{{- end }}
{{- end }}

{{/*
Return the kubectl port-forward command for querying profiles from Pyroscope.
*/}}
{{- define "pyroscope.kubectl_port_forward_read" -}}
{{- $components := (fromYaml (include "pyroscope.components" .)) -}}
{{- if .Values.architecture.deployUnifiedServices -}}
kubectl --namespace {{ .Release.Namespace }} port-forward svc/{{ include "pyroscope.fullname" . }}-read {{ .Values.pyroscope.service.port }}:80
{{- else if hasKey $components "query-frontend" -}}
{{- $port := ((index $components "query-frontend").service).port | default .Values.pyroscope.service.port }}
kubectl --namespace {{ .Release.Namespace }} port-forward svc/{{ include "pyroscope.fullname" . }}-query-frontend {{ $port }}:{{ $port }}
{{- else -}}
kubectl --namespace {{ .Release.Namespace }} port-forward svc/{{ include "pyroscope.fullname" . }} {{ .Values.pyroscope.service.port }}:{{ .Values.pyroscope.service.port }}
{{- end }}
{{- end }}

{{- define "pyroscope.defaultAutoscalingComponents" -}}
enabled: false
minReplicas: 1
maxReplicas: 3
targetCPUUtilizationPercentage: 60
targetMemoryUtilizationPercentage: null
customMetrics: []
behavior:
    enabled: false
    scaleDown: {}
    scaleUp: {}
{{- end }}

{{/*
Fail on invalid .Values.architecture.storage settings. Renders nothing.
*/}}
{{- define "pyroscope.validateArchitecture" -}}
{{- $storage := .Values.architecture.storage }}
{{- if and (not $storage.v1) (not $storage.v2) }}
{{-   fail "you need to set at least one of .Values.architecture.storage.v1, .Values.architecture.storage.v2 to true" }}
{{- end }}
{{- /* migration settings are only used when v1 and v2 run together */}}
{{- if and $storage.v1 $storage.v2 }}
{{-   $migration := $storage.migration }}
{{-   range $key := list "ingesterWeight" "segmentWriterWeight" }}
{{-     $weight := index $migration $key }}
{{- /* helm --set keeps floats like 0.1 as strings, so match the text form */}}
{{-     if not (regexMatch "^[+-]?(\\d+\\.?\\d*|\\.\\d+)([eE][+-]?\\d+)?$" (toString $weight)) }}
{{-       fail (printf ".Values.architecture.storage.migration.%s must be a number between 0 and 1, got %q" $key (toString $weight)) }}
{{-     end }}
{{-     if or (lt (float64 $weight) 0.0) (gt (float64 $weight) 1.0) }}
{{-       fail (printf ".Values.architecture.storage.migration.%s must be between 0 and 1, got %v" $key $weight) }}
{{-     end }}
{{-   end }}
{{- /* each write goes to a path when rand[0,1) <= its weight, so one weight must be 1 to keep every write */}}
{{-   if and (lt (float64 $migration.ingesterWeight) 1.0) (lt (float64 $migration.segmentWriterWeight) 1.0) }}
{{-     fail (printf "at least one of .Values.architecture.storage.migration.ingesterWeight, .Values.architecture.storage.migration.segmentWriterWeight must be 1, otherwise some writes are dropped (got %v and %v)" $migration.ingesterWeight $migration.segmentWriterWeight) }}
{{-   end }}
{{-   $from := toString ($migration.queryBackendFrom | default "auto") }}
{{-   if not (regexMatch "^(auto|0|\\d{4}-\\d{2}-\\d{2}(T\\d{2}:\\d{2}(:\\d{2}(\\.\\d+)?(Z|[+-]\\d{2}:\\d{2}))?)?)$" $from) }}
{{-     fail (printf ".Values.architecture.storage.migration.queryBackendFrom must be \"auto\" or a time like 2006-01-02, 2006-01-02T15:04 or 2006-01-02T15:04:05Z07:00, got %q" $from) }}
{{-   end }}
{{- end }}
{{- end }}
