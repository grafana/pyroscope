{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='runtimeClassStrategyOptions', url='', help='"RuntimeClassStrategyOptions define the strategy that will dictate the allowable RuntimeClasses for a pod."'),
  '#withAllowedRuntimeClassNames':: d.fn(help='"allowedRuntimeClassNames is an allowlist of RuntimeClass names that may be specified on a pod. A value of \\"*\\" means that any RuntimeClass name is allowed, and must be the only item in the list. An empty list requires the RuntimeClassName field to be unset."', args=[d.arg(name='allowedRuntimeClassNames', type=d.T.array)]),
  withAllowedRuntimeClassNames(allowedRuntimeClassNames): { allowedRuntimeClassNames: if std.isArray(v=allowedRuntimeClassNames) then allowedRuntimeClassNames else [allowedRuntimeClassNames] },
  '#withAllowedRuntimeClassNamesMixin':: d.fn(help='"allowedRuntimeClassNames is an allowlist of RuntimeClass names that may be specified on a pod. A value of \\"*\\" means that any RuntimeClass name is allowed, and must be the only item in the list. An empty list requires the RuntimeClassName field to be unset."\n\n**Note:** This function appends passed data to existing values', args=[d.arg(name='allowedRuntimeClassNames', type=d.T.array)]),
  withAllowedRuntimeClassNamesMixin(allowedRuntimeClassNames): { allowedRuntimeClassNames+: if std.isArray(v=allowedRuntimeClassNames) then allowedRuntimeClassNames else [allowedRuntimeClassNames] },
  '#withDefaultRuntimeClassName':: d.fn(help='"defaultRuntimeClassName is the default RuntimeClassName to set on the pod. The default MUST be allowed by the allowedRuntimeClassNames list. A value of nil does not mutate the Pod."', args=[d.arg(name='defaultRuntimeClassName', type=d.T.string)]),
  withDefaultRuntimeClassName(defaultRuntimeClassName): { defaultRuntimeClassName: defaultRuntimeClassName },
  '#mixin': 'ignore',
  mixin: self,
}
