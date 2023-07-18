local d = import 'doc-util/main.libsonnet';

local patch = {
  cronJob+: {
    '#new'+: d.func.withArgs([
      d.arg('name', d.T.string),
      d.arg('schedule', d.T.string),
      d.arg('containers', d.T.array),
    ]),
    new(
      name,
      schedule='',
      containers=[]
    )::
      super.new(name)
      + super.spec.withSchedule(schedule)
      + super.spec.jobTemplate.spec.template.spec.withContainers(containers)
      + super.spec.jobTemplate.spec.template.metadata.withLabels({ name: name }),
  },
};

{
  batch+: {
    v1+: patch,
    v1beta1+: patch,
    v2alpha1+: patch,
  },
}
