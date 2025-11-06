#!/usr/bin/env bash

# Goal of that script is to convert a grafana dashboard from internal GL environments so it can be used with this pyroscope-monitoring chart.

set -eux
set -o pipefail

# loop over inputs
for dashboard in "$@"
do
	filename="$(basename $dashboard)"
	name="${filename%.*}"
	destination="templates/_dashboard_${name}.yaml"

	# create basic dashboard
	echo "{{- define \"pyroscope-monitoring.dashboards.${name}\" -}}" > "${destination}"
	echo "{{- \$dashboard := \"${name}\" -}}" >> "${destination}"
	cat "$dashboard" | js-yaml | sed 's/{{.*}}/{{"\0\"}}/g' >> "${destination}"
	echo "{{- end }}" >> "${destination}"

	# replace specific namespace
	sed -i 's/profiles-ops-002/{{.Values.dashboards.namespace | quote}}/g' "${destination}"
	sed -i 's/fire|profiles-\.\*/{{.Values.dashboards.namespaceRegex | quote}}/g' "${destination}"
	sed -i 's/fire-\.\*|profiles-\.\*/{{.Values.dashboards.namespaceRegex | quote}}/g' "${destination}"

	# replace default cluster
	sed -i 's/ops-eu-south-0/{{.Values.dashboards.cluster | quote}}/g' "${destination}"

	# replace cortex-gw selectors
	sed -i 's/{job=~"$namespace\/cortex-gw(-internal)?"/{ {{ include "pyroscope-monitoring.dashboards-ingest-selector" . }}, namespace=~"$namespace"/g' "${destination}"
	sed -i 's/job=~"$namespace\/cortex-gw(-internal)?"/{{ include "pyroscope-monitoring.dashboards-ingest-selector" . }}, namespace=~"$namespace"/g' "${destination}"

	# replace cadvisor selectors
	sed -i 's/{job="kube-system\/cadvisor"/{ {{ .Values.dashboards.cadvisorSelector }}/g' "${destination}"
	sed -i 's/job="kube-system\/cadvisor"/{{ .Values.dashboards.cadvisorSelector }}/g' "${destination}"

	# replace kube-state-metrics selectors
	sed -i 's/{job="kube-system\/kube-state-metrics"/{ {{ .Values.dashboards.kubeStateMetricsSelector }}/g' "${destination}"
	sed -i 's/job="kube-system\/kube-state-metrics"/{{ .Values.dashboards.kubeStateMetricsSelector }}/g' "${destination}"
	sed -i 's/{job="kube-state-metrics\/kube-state-metrics"/{ {{ .Values.dashboards.kubeStateMetricsSelector }}/g' "${destination}"
	sed -i 's/job="kube-state-metrics\/kube-state-metrics"/{{ .Values.dashboards.kubeStateMetricsSelector }}/g' "${destination}"

	# fix extra namespace
	sed -i 's/{ {{/{{ "{" }}{{/g' "${destination}"
done
