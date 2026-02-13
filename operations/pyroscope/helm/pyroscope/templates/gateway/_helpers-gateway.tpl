{{/*
gateway fullname
*/}}
{{- define "pyroscope.gatewayFullname" -}}
{{ include "pyroscope.fullname" . }}-gateway
{{- end }}

{{/*
gateway common labels
*/}}
{{- define "pyroscope.gatewayLabels" -}}
{{ include "pyroscope.labels" . }}
app.kubernetes.io/component: pyroscope-gateway
{{- end }}

{{/*
gateway selector labels
*/}}
{{- define "pyroscope.gatewaySelectorLabels" -}}
{{ include "pyroscope.selectorLabels" . }}
app.kubernetes.io/component: pyroscope-gateway
{{- end }}

{{/*
gateway auth secret name
*/}}
{{- define "pyroscope.gatewayAuthSecret" -}}
{{ .Values.gateway.basicAuth.existingSecret | default (include "pyroscope.gatewayFullname" . ) }}
{{- end }}

{{/*
gateway Docker image
*/}}
{{- define "pyroscope.gatewayImage" -}}
{{- $dict := dict "service" .Values.gateway.image -}}
{{- include "pyroscope.baseImage" $dict -}}
{{- end }}

{{/*
gateway priority class name
*/}}
{{- define "pyroscope.gatewayPriorityClassName" -}}
{{- $pcn := .Values.gateway.priorityClassName -}}
{{- if $pcn }}
priorityClassName: {{ $pcn }}
{{- end }}
{{- end }}
