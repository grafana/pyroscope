local tanka = import 'github.com/grafana/jsonnet-libs/tanka-util/main.libsonnet';
local helm = tanka.helm.new(std.thisFile);

local defaults = {
  namespace: 'monitoring',
  pyroscope: {
    values: {
      persistence: { enabled: true },
    },
  },
};

{
  new(name='pyroscope', overrides={})::
    helm.template(name, '../../helm/pyroscope', defaults + overrides),
}
