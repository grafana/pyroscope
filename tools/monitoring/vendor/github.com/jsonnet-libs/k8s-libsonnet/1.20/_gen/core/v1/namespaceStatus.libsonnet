{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='namespaceStatus', url='', help='"NamespaceStatus is information about the current status of a Namespace."'),
  '#withConditions':: d.fn(help="\"Represents the latest available observations of a namespace's current state.\"", args=[d.arg(name='conditions', type=d.T.array)]),
  withConditions(conditions): { conditions: if std.isArray(v=conditions) then conditions else [conditions] },
  '#withConditionsMixin':: d.fn(help="\"Represents the latest available observations of a namespace's current state.\"\n\n**Note:** This function appends passed data to existing values", args=[d.arg(name='conditions', type=d.T.array)]),
  withConditionsMixin(conditions): { conditions+: if std.isArray(v=conditions) then conditions else [conditions] },
  '#withPhase':: d.fn(help='"Phase is the current lifecycle phase of the namespace. More info: https://kubernetes.io/docs/tasks/administer-cluster/namespaces/"', args=[d.arg(name='phase', type=d.T.string)]),
  withPhase(phase): { phase: phase },
  '#mixin': 'ignore',
  mixin: self,
}
