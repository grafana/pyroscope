{
  local this = self,

  local configTemplate = {
    description: |||
      [{{ .Status | toUpper }}{{ if eq .Status "firing" }}:{{ .Alerts.Firing | len }}{{ end }}] {{ .GroupLabels.cluster }}: {{ .GroupLabels.alertname }} ({{ .GroupLabels.namespace }})
      {{ .CommonAnnotations.summary }}
      {{ if .Alerts.Firing | len }}Firing alerts:
      {{ range .Alerts.Firing }}- {{ .Annotations.message }}{{ .Annotations.description }}
      {{ end }}{{ end }}{{ if .Alerts.Resolved | len }}Resolved alerts:
      {{ range .Alerts.Resolved }}- {{ .Annotations.message }}{{ .Annotations.description }}
      {{ end }}{{ end }}
    |||,
    details: {
      firing: |||
        {{ with .Alerts.Firing }}
        {{ range . }}Labels:
        {{ range .Labels.SortedPairs }} - {{ .Name }} = {{ .Value }}
        {{ end }}Annotations:
        {{ range .Annotations.SortedPairs }} - {{ .Name }} = {{ .Value }}
        {{ end }}Source: {{ .GeneratorURL }}
        {{ end }}{{ end }}
        Silence: {{ template "__alert_silence_link" . }}
      |||,
    },
  },

  new(name): this.withName(name),

  withName(name):: {
    name: name,
  },

  withConfig(config):: {
    pagerduty_configs: [config],
  },

  withConfigMixin(config):: {
    pagerduty_configs+: [config],
  },


  config: {
    newService(service_key): {
      service_key: service_key,
    } + this.config.withConfigTemplate(),

    newRouting(routing_key): {
      routing_key: routing_key,
    } + this.config.withConfigTemplate(),

    withConfigTemplate(): configTemplate,
  },
}
