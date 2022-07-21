{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='apiServiceStatus', url='', help='"APIServiceStatus contains derived information about an API server"'),
  '#withConditions':: d.fn(help='"Current service state of apiService."', args=[d.arg(name='conditions', type=d.T.array)]),
  withConditions(conditions): { conditions: if std.isArray(v=conditions) then conditions else [conditions] },
  '#withConditionsMixin':: d.fn(help='"Current service state of apiService."\n\n**Note:** This function appends passed data to existing values', args=[d.arg(name='conditions', type=d.T.array)]),
  withConditionsMixin(conditions): { conditions+: if std.isArray(v=conditions) then conditions else [conditions] },
  '#mixin': 'ignore',
  mixin: self,
}
