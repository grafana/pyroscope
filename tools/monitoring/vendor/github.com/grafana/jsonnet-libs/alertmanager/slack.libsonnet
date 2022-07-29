// This file is for backwards compat, please use alertmanager/receivers.libsonnet instead.
local slack = import './receivers/slack.libsonnet';

{
  local this = self,

  _config+:: {
    slack_url: 'http://slack',
    slack_channel: 'general',
  },

  build_slack_receiver(name, slack_channel)::
    slack.new(name, slack_channel)
    + slack.withAPIUrl(this._config.slack_url),

  alertmanager_config+:: {
    route+: {
      group_by: ['alertname'],
      receiver: 'slack',
    },

    receivers+: [
      this.build_slack_receiver('slack', this._config.slack_channel),
    ],
  },
}
