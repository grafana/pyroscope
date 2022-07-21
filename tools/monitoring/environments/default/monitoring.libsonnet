local k = import 'ksonnet-util/kausal.libsonnet';
local prometheus = import 'prometheus-ksonnet/prometheus-ksonnet.libsonnet';
local fire = import './../../../../deploy/jsonnet/fire-mixin/mixin.libsonnet';


{
  local deployment = k.apps.v1.deployment,
  local container = k.core.v1.container,
  local port = k.core.v1.containerPort,
  local service = k.core.v1.service,
  prometheus: prometheus {
    mixins+::  {
      fire: fire,
    },
    _config+:: {
      cluster_name: 'default',
      namespace: 'monitoring',
    },
  },
}
