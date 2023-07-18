{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='labelSelector', url='', help='"A label selector is a label query over a set of resources. The result of matchLabels and matchExpressions are ANDed. An empty label selector matches all objects. A null label selector matches no objects."'),
  '#withMatchExpressions':: d.fn(help='"matchExpressions is a list of label selector requirements. The requirements are ANDed."', args=[d.arg(name='matchExpressions', type=d.T.array)]),
  withMatchExpressions(matchExpressions): { matchExpressions: if std.isArray(v=matchExpressions) then matchExpressions else [matchExpressions] },
  '#withMatchExpressionsMixin':: d.fn(help='"matchExpressions is a list of label selector requirements. The requirements are ANDed."\n\n**Note:** This function appends passed data to existing values', args=[d.arg(name='matchExpressions', type=d.T.array)]),
  withMatchExpressionsMixin(matchExpressions): { matchExpressions+: if std.isArray(v=matchExpressions) then matchExpressions else [matchExpressions] },
  '#withMatchLabels':: d.fn(help='"matchLabels is a map of {key,value} pairs. A single {key,value} in the matchLabels map is equivalent to an element of matchExpressions, whose key field is \\"key\\", the operator is \\"In\\", and the values array contains only \\"value\\". The requirements are ANDed."', args=[d.arg(name='matchLabels', type=d.T.object)]),
  withMatchLabels(matchLabels): { matchLabels: matchLabels },
  '#withMatchLabelsMixin':: d.fn(help='"matchLabels is a map of {key,value} pairs. A single {key,value} in the matchLabels map is equivalent to an element of matchExpressions, whose key field is \\"key\\", the operator is \\"In\\", and the values array contains only \\"value\\". The requirements are ANDed."\n\n**Note:** This function appends passed data to existing values', args=[d.arg(name='matchLabels', type=d.T.object)]),
  withMatchLabelsMixin(matchLabels): { matchLabels+: matchLabels },
  '#mixin': 'ignore',
  mixin: self,
}
