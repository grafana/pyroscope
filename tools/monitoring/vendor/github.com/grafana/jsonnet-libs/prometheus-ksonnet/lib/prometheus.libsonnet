local prometheus_mixins = import 'prometheus/mixins.libsonnet';
local prometheus = import 'prometheus/prometheus.libsonnet';

prometheus_mixins {
  /*
   * All Prometheus resources are contained within a `prometheus` node. This allows
     multiple Prometheus instances to be created by simply cloning this node, like
     so:
     `other_prometheus: $.prometheus {name: "other-prometheus"},`

     To remove the default Prometheus, use this code:
     `main_prometheus: {},`
  */

  prometheus:: prometheus {
    name:: error 'must specify name',
    local name = self.name,

    _config+:: $._config { name: name },
    _images+:: $._images,
    mixins:: $.mixins,
    prometheusRules:: if std.objectHas($, 'prometheusRules') then $.prometheusRules else {},
    prometheusAlerts:: if std.objectHas($, 'prometheusAlerts') then $.prometheusAlerts else {},
    prometheus_config+:: $.prometheus_config,

  },

  main_prometheus: $.prometheus { name: 'prometheus' },
}
