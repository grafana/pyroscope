{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='flowSchemaSpec', url='', help="\"FlowSchemaSpec describes how the FlowSchema's specification looks like.\""),
  '#distinguisherMethod':: d.obj(help='"FlowDistinguisherMethod specifies the method of a flow distinguisher."'),
  distinguisherMethod: {
    '#withType':: d.fn(help='"`type` is the type of flow distinguisher method The supported types are \\"ByUser\\" and \\"ByNamespace\\". Required."', args=[d.arg(name='type', type=d.T.string)]),
    withType(type): { distinguisherMethod+: { type: type } },
  },
  '#priorityLevelConfiguration':: d.obj(help='"PriorityLevelConfigurationReference contains information that points to the \\"request-priority\\" being used."'),
  priorityLevelConfiguration: {
    '#withName':: d.fn(help='"`name` is the name of the priority level configuration being referenced Required."', args=[d.arg(name='name', type=d.T.string)]),
    withName(name): { priorityLevelConfiguration+: { name: name } },
  },
  '#withMatchingPrecedence':: d.fn(help='"`matchingPrecedence` is used to choose among the FlowSchemas that match a given request. The chosen FlowSchema is among those with the numerically lowest (which we take to be logically highest) MatchingPrecedence.  Each MatchingPrecedence value must be ranged in [1,10000]. Note that if the precedence is not specified, it will be set to 1000 as default."', args=[d.arg(name='matchingPrecedence', type=d.T.integer)]),
  withMatchingPrecedence(matchingPrecedence): { matchingPrecedence: matchingPrecedence },
  '#withRules':: d.fn(help='"`rules` describes which requests will match this flow schema. This FlowSchema matches a request if and only if at least one member of rules matches the request. if it is an empty slice, there will be no requests matching the FlowSchema."', args=[d.arg(name='rules', type=d.T.array)]),
  withRules(rules): { rules: if std.isArray(v=rules) then rules else [rules] },
  '#withRulesMixin':: d.fn(help='"`rules` describes which requests will match this flow schema. This FlowSchema matches a request if and only if at least one member of rules matches the request. if it is an empty slice, there will be no requests matching the FlowSchema."\n\n**Note:** This function appends passed data to existing values', args=[d.arg(name='rules', type=d.T.array)]),
  withRulesMixin(rules): { rules+: if std.isArray(v=rules) then rules else [rules] },
  '#mixin': 'ignore',
  mixin: self,
}
