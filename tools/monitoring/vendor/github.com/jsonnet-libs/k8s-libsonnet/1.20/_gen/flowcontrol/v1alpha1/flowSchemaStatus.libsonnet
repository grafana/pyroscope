{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='flowSchemaStatus', url='', help='"FlowSchemaStatus represents the current state of a FlowSchema."'),
  '#withConditions':: d.fn(help='"`conditions` is a list of the current states of FlowSchema."', args=[d.arg(name='conditions', type=d.T.array)]),
  withConditions(conditions): { conditions: if std.isArray(v=conditions) then conditions else [conditions] },
  '#withConditionsMixin':: d.fn(help='"`conditions` is a list of the current states of FlowSchema."\n\n**Note:** This function appends passed data to existing values', args=[d.arg(name='conditions', type=d.T.array)]),
  withConditionsMixin(conditions): { conditions+: if std.isArray(v=conditions) then conditions else [conditions] },
  '#mixin': 'ignore',
  mixin: self,
}
