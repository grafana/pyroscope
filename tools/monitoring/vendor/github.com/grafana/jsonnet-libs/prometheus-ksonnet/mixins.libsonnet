{
  // Add you mixins here.
  mixins+:: {
    base+: {
      prometheusRules+::
        {
          groups+: [
            {
              // Add mapping from namespace, pod -> node with node name as pod, as
              // we use the node name as the node-exporter instance label.
              name: 'instance_override',
              rules: [
                {
                  record: 'node_namespace_pod:kube_pod_info:',
                  expr: |||
                    max by(node, namespace, instance) (
                      label_replace(kube_pod_info{job="%(kube_state_metrics_namespace)s/kube-state-metrics"}
                      , "instance", "$1", "node", "(.*)")
                    )
                  ||| % $._config,
                },
              ],
            },
          ],
        },
    },

    kubernetes:
      (import 'kubernetes-mixin/mixin.libsonnet') {
        grafanaDashboardFolder: 'Kubernetes',

        _config+:: {
          cadvisorSelector: 'job="kube-system/cadvisor"',
          kubeletSelector: 'job="kube-system/kubelet"',
          kubeStateMetricsSelector: 'job="%s/kube-state-metrics"' % $._config.kube_state_metrics_namespace,
          nodeExporterSelector: 'job="%s/node-exporter"' % $._config.node_exporter_namespace,  // Also used by node-mixin.
          notKubeDnsSelector: 'job!="kube-system/kube-dns"',
          kubeSchedulerSelector: 'job="kube-system/kube-scheduler"',
          kubeControllerManagerSelector: 'job="kube-system/kube-controller-manager"',
          kubeApiserverSelector: 'job="kube-system/kube-apiserver"',
          podLabel: 'instance',
          notKubeDnsCoreDnsSelector: 'job!~"kube-system/kube-dns|coredns"',
        },
      },

    prometheus:
      (import 'prometheus-mixin/mixin.libsonnet') {
        grafanaDashboardFolder: 'Prometheus',

        _config+:: {
          prometheusSelector: 'job="default/prometheus"',
          prometheusHAGroupLabels: 'job,cluster,namespace',
          prometheusHAGroupName: '{{$labels.job}} in {{$labels.cluster}}',
        },
      },

    alertmanager:
      (import 'alertmanager-mixin/mixin.libsonnet') {
        grafanaDashboardFolder: 'Alertmanager',

        _config+:: {
          alertmanagerSelector: 'job="default/alertmanager"',
          alertmanagerClusterLabels: 'job, namespace',
          alertmanagerName: '{{$labels.instance}} in {{$labels.cluster}}',
          alertmanagerCriticalIntegrationsRegEx: @'pagerduty',
        },
      },

    node_exporter:
      (import 'node-mixin/mixin.libsonnet') {
        grafanaDashboardFolder: 'node_exporter',

        _config+:: {
          nodeExporterSelector: 'job="%s/node-exporter"' % $._config.node_exporter_namespace,  // Also used by node-mixin.

          // Do not page if nodes run out of disk space.
          nodeCriticalSeverity: 'warning',
          grafanaPrefix: '/grafana',
        },
      },

    // A more complete view than the node_exporter
    node_exporter_full: {
      grafanaDashboardFolder: 'node_exporter_full',
      grafanaDashboards+:: {
        'node-exporter-full.json': (import 'github.com/rfrail3/grafana-dashboards/prometheus/node-exporter-full.json'),
      },
    },

    grafana:
      (import 'grafana-mixin/mixin.libsonnet'),
  },
}
