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
Template labels
*/}}
{{- define "pyroscope.templateLabels" -}}
{{ include "pyroscope.selectorLabels" . }}
{{- range $k, $v := .Values.pyroscope.extraLabels }}
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
{{- $components = mustMergeOverwrite (deepCopy $components) (.Values.pyroscope.components | default dict)}}
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
{{- else if hasKey $components "query-frontend" -}}A
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


{{/* Allow KubeVersion to be overridden. */}}
{{- define "pyroscope.kubeVersion" -}}
  {{- default .Capabilities.KubeVersion.Version .Values.kubeVersionOverride -}}
{{- end -}}

{{/*
Return the appropriate apiVersion for ingress.
*/}}
{{- define "pyroscope.ingress.apiVersion" -}}
  {{- if and (.Capabilities.APIVersions.Has "networking.k8s.io/v1") (semverCompare ">= 1.19-0" (include "pyroscope.kubeVersion" .)) -}}
      {{- print "networking.k8s.io/v1" -}}
  {{- else if .Capabilities.APIVersions.Has "networking.k8s.io/v1beta1" -}}
    {{- print "networking.k8s.io/v1beta1" -}}
  {{- else -}}
    {{- print "extensions/v1beta1" -}}
  {{- end -}}
{{- end -}}

{/*
Return if ingress is stable.
*/}}
{{- define "pyroscope.ingress.isStable" -}}
  {{- eq (include "pyroscope.ingress.apiVersion" .) "networking.k8s.io/v1" -}}
{{- end -}}

{{/*
Return if ingress supports ingressClassName.
*/}}
{{- define "pyroscope.ingress.supportsIngressClassName" -}}
  {{- or (eq (include "pyroscope.ingress.isStable" .) "true") (and (eq (include "pyroscope.ingress.apiVersion" .) "networking.k8s.io/v1beta1") (semverCompare ">= 1.18-0" (include "pyroscope.kubeVersion" .))) -}}
{{- end -}}

{{/*
Return if ingress supports pathType.
*/}}
{{- define "pyroscope.ingress.supportsPathType" -}}
  {{- or (eq (include "pyroscope.ingress.isStable" .) "true") (and (eq (include "pyroscope.ingress.apiVersion" .) "networking.k8s.io/v1beta1") (semverCompare ">= 1.18-0" (include "pyroscope.kubeVersion" .))) -}}
{{- end -}}

{{/*
compute a ConfigMap or Secret checksum only based on its .data content.
This function needs to be called with a context object containing the following keys:
- ctx: the current Helm context (what '.' is at the call site)
- name: the file name of the ConfigMap or Secret
*/}}
{{- define "pyroscope.configMapOrSecretContentHash" -}}
{{ get (include (print .ctx.Template.BasePath .name) .ctx | fromYaml) "data" | toYaml | sha256sum }}
{{- end }}

{{/* Configure enableServiceLinks in pod */}}
{{- define "pyroscope.enableServiceLinks" -}}
{{- if semverCompare ">=1.13-0" (include "pyroscope.kubeVersion" .) -}}
{{- if or (.Values.pyroscope.enableServiceLinks) (ne .Values.pyroscope.enableServiceLinks false) -}}
enableServiceLinks: true
{{- else -}}
enableServiceLinks: false
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Base template for building docker image reference
*/}}
{{- define "pyroscope.baseImage" }}
{{- $registry := .service.registry | default "" -}}
{{- $repository := .service.repository | default "" -}}
{{- $ref := ternary (printf ":%s" (.service.tag | default .defaultVersion | toString)) (printf "@%s" .service.digest) (empty .service.digest) -}}
{{- if and $registry $repository -}}
  {{- printf "%s/%s%s" $registry $repository $ref -}}
{{- else -}}
  {{- printf "%s%s%s" $registry $repository $ref -}}
{{- end -}}
{{- end -}}
