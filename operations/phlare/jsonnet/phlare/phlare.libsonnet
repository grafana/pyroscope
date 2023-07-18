local tanka = import 'github.com/grafana/jsonnet-libs/tanka-util/main.libsonnet';
local helm = tanka.helm.new(std.thisFile);

local defaults = {
  namespace: 'monitoring',
  phlare: {
    values: {
      persistence: { enabled: true },
    },
  },
};

{
  new(name='phlare', overrides={})::
    helm.template(name, '../../helm/phlare', defaults + overrides),
}
