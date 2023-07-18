{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='v1', url='', help=''),
  mutatingWebhook: (import 'mutatingWebhook.libsonnet'),
  mutatingWebhookConfiguration: (import 'mutatingWebhookConfiguration.libsonnet'),
  ruleWithOperations: (import 'ruleWithOperations.libsonnet'),
  serviceReference: (import 'serviceReference.libsonnet'),
  validatingWebhook: (import 'validatingWebhook.libsonnet'),
  validatingWebhookConfiguration: (import 'validatingWebhookConfiguration.libsonnet'),
  webhookClientConfig: (import 'webhookClientConfig.libsonnet'),
}
