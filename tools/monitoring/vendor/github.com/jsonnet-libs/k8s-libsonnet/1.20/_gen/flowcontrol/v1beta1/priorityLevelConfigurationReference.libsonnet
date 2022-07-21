{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='priorityLevelConfigurationReference', url='', help='"PriorityLevelConfigurationReference contains information that points to the \\"request-priority\\" being used."'),
  '#withName':: d.fn(help='"`name` is the name of the priority level configuration being referenced Required."', args=[d.arg(name='name', type=d.T.string)]),
  withName(name): { name: name },
  '#mixin': 'ignore',
  mixin: self,
}
