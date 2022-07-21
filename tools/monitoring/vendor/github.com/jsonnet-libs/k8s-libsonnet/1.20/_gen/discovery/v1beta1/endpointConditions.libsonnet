{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='endpointConditions', url='', help='"EndpointConditions represents the current condition of an endpoint."'),
  '#withReady':: d.fn(help='"ready indicates that this endpoint is prepared to receive traffic, according to whatever system is managing the endpoint. A nil value indicates an unknown state. In most cases consumers should interpret this unknown state as ready. For compatibility reasons, ready should never be \\"true\\" for terminating endpoints."', args=[d.arg(name='ready', type=d.T.boolean)]),
  withReady(ready): { ready: ready },
  '#withServing':: d.fn(help='"serving is identical to ready except that it is set regardless of the terminating state of endpoints. This condition should be set to true for a ready endpoint that is terminating. If nil, consumers should defer to the ready condition. This field can be enabled with the EndpointSliceTerminatingCondition feature gate."', args=[d.arg(name='serving', type=d.T.boolean)]),
  withServing(serving): { serving: serving },
  '#withTerminating':: d.fn(help='"terminating indicates that this endpoint is terminating. A nil value indicates an unknown state. Consumers should interpret this unknown state to mean that the endpoint is not terminating. This field can be enabled with the EndpointSliceTerminatingCondition feature gate."', args=[d.arg(name='terminating', type=d.T.boolean)]),
  withTerminating(terminating): { terminating: terminating },
  '#mixin': 'ignore',
  mixin: self,
}
