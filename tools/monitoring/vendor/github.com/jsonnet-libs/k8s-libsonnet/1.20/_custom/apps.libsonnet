local d = import 'doc-util/main.libsonnet';
local gen = import '../gen.libsonnet';

local patch = {
  daemonSet+: {
    '#new'+: d.func.withArgs([
      d.arg('name', d.T.string),
      d.arg('containers', d.T.array),
      d.arg('podLabels', d.T.object, {}),
    ]),
    new(
      name,
      containers=[],
      podLabels={}
    )::
      local labels = { name: name } + podLabels;
      super.new(name)
      + super.spec.template.spec.withContainers(containers)
      + super.spec.template.metadata.withLabels(labels)
      + super.spec.selector.withMatchLabels(labels),
  },
  deployment+: {
    '#new'+: d.func.withArgs([
      d.arg('name', d.T.string),
      d.arg('replicas', d.T.int, 1),
      d.arg('containers', d.T.array),
      d.arg('podLabels', d.T.object, {}),
    ]),
    new(
      name,
      replicas=1,
      containers=error 'containers unset',
      podLabels={},
    )::
      local labels = { name: name } + podLabels;
      super.new(name)
      + super.spec.withReplicas(replicas)
      + super.spec.template.spec.withContainers(containers)
      + super.spec.template.metadata.withLabels(labels)
      + super.spec.selector.withMatchLabels(labels),
  },

  statefulSet+: {
    '#new'+: d.func.withArgs([
      d.arg('name', d.T.string),
      d.arg('replicas', d.T.int, 1),
      d.arg('containers', d.T.array),
      d.arg('volumeClaims', d.T.array, []),
      d.arg('podLabels', d.T.object, {}),
    ]),
    new(
      name,
      replicas=1,
      containers=error 'containers unset',
      volumeClaims=[],
      podLabels={},
    )::
      local labels = { name: name } + podLabels;
      super.new(name)
      + super.spec.withReplicas(replicas)
      + super.spec.template.spec.withContainers(containers)
      + super.spec.template.metadata.withLabels(labels)
      + super.spec.selector.withMatchLabels(labels)

      // remove volumeClaimTemplates if empty
      // (otherwise it will create a diff all the time)
      + (
        if std.length(volumeClaims) > 0
        then super.spec.withVolumeClaimTemplates(volumeClaims)
        else {}
      ),
  },
};

{
  [if std.objectHas(gen, 'extensions') then 'extensions']+: { // This was removed in v1.22
    [if std.objectHas(gen.extensions, 'v1beta1') then 'v1beta1']+: patch,
  },
  apps+: {
    v1+: patch,
    v1beta1+: patch,
    v1beta2+: patch,
  },
}
