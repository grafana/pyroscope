{
  local this = self,

  new(name, channel):
    this.withName(name)
    + this.withConfig(this.config.new(channel)),

  withName(name):: {
    name: name,
  },

  withConfig(config):: {
    slack_configs: [config],
  },

  withConfigMixin(config):: {
    slack_configs+: [config],
  },

  withAPIUrl(api_url):: {
    slack_configs: [
      config + this.config.withAPIUrl(api_url)
      for config in super.slack_configs
    ],
  },

  withAPIUrlFile(api_url_file):: {
    slack_configs: [
      config + this.config.withAPIUrlFile(api_url_file)
      for config in super.slack_configs
    ],
  },

  config: {
    new(channel): {
      channel: channel,
      send_resolved: true,
      title: '{{ template "__alert_title" . }}',
      text: '{{ template "__alert_text" . }}',
      actions: [
        {
          type: 'button',
          text: 'Runbook :green_book:',
          url: '{{ (index .Alerts 0).Annotations.runbook_url }}',
        },

        {
          type: 'button',
          text: 'Source :information_source:',
          url: '{{ (index .Alerts 0).GeneratorURL }}',
        },

        {
          type: 'button',
          text: 'Silence :no_bell:',
          url: '{{ template "__alert_silence_link" . }}',
        },

        {
          type: 'button',
          text: 'Dashboard :grafana:',
          url: '{{ (index .Alerts 0).Annotations.dashboard_url }}',
        },

        {
          type: 'button',
          text: 'logs :lokii:',
          url: '{{ (index .Alerts 0).Annotations.logs_url }}',
        },
      ],
    },

    withAPIUrl(api_url): {
      api_url: api_url,
    },

    withAPIUrlFile(api_url_file): {
      api_url_file: api_url_file,
    },
  },
}
