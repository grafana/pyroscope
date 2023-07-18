{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='hpaScalingPolicy', url='', help='"HPAScalingPolicy is a single policy which must hold true for a specified past interval."'),
  '#withPeriodSeconds':: d.fn(help='"PeriodSeconds specifies the window of time for which the policy should hold true. PeriodSeconds must be greater than zero and less than or equal to 1800 (30 min)."', args=[d.arg(name='periodSeconds', type=d.T.integer)]),
  withPeriodSeconds(periodSeconds): { periodSeconds: periodSeconds },
  '#withType':: d.fn(help='"Type is used to specify the scaling policy."', args=[d.arg(name='type', type=d.T.string)]),
  withType(type): { type: type },
  '#withValue':: d.fn(help='"Value contains the amount of change which is permitted by the policy. It must be greater than zero"', args=[d.arg(name='value', type=d.T.integer)]),
  withValue(value): { value: value },
  '#mixin': 'ignore',
  mixin: self,
}
