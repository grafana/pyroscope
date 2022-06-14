local tanka = import 'github.com/grafana/jsonnet-libs/tanka-util/main.libsonnet';
local helm = tanka.helm.new(std.thisFile);

local defaults = {
  namespace: 'monitoring',
  fire: {
    values: {
      persistence: { enabled: true },
    },
  },
};

{
  new(name='fire', overrides={})::
    helm.template(name, '../../helm/fire', defaults + overrides),
}
