{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='eventSeries', url='', help='"EventSeries contain information on series of events, i.e. thing that was/is happening continuously for some time. How often to update the EventSeries is up to the event reporters. The default event reporter in \\"k8s.io/client-go/tools/events/event_broadcaster.go\\" shows how this struct is updated on heartbeats and can guide customized reporter implementations."'),
  '#withCount':: d.fn(help='"count is the number of occurrences in this series up to the last heartbeat time."', args=[d.arg(name='count', type=d.T.integer)]),
  withCount(count): { count: count },
  '#withLastObservedTime':: d.fn(help='"MicroTime is version of Time with microsecond level precision."', args=[d.arg(name='lastObservedTime', type=d.T.string)]),
  withLastObservedTime(lastObservedTime): { lastObservedTime: lastObservedTime },
  '#mixin': 'ignore',
  mixin: self,
}
