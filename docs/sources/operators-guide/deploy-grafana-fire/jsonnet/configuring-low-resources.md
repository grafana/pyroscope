---
aliases:
  - /docs/fire/latest/operators-guide/deploying-grafana-fire/jsonnet/configuring-low-resources/
description: Learn how to configure Grafana Fire when using Jsonnet.
menuTitle: Configuring low resources
title: Configuring Grafana Fire to use low resources with Jsonnet
weight: 20
---

# Configuring Grafana Fire to use low resources with Jsonnet

This page describes how to configure Jsonnet to deploy Grafana Fire in a Kubernetes cluster with low CPU and memory resources available.

## Anti-affinity

Given the distributed nature of Fire, both performance and reliability are improved when pods are spread across different nodes.
For example, losing multiple ingesters can cause data loss, so it's better to distribute them across different nodes.

For this reason, by default, anti-affinity rules are applied to some Kubernetes Deployments and StatefulSets.
These anti-affinity rules can become an issue when playing with Fire in a single-node Kubernetes cluster.
You can disable anti-affinity by setting the configuration values `_config.<component>_allow_multiple_replicas_on_same_node`.

### Example: disable anti-affinity

```jsonnet
local fire = import 'fire/fire.libsonnet';

fire {
  _config+:: {
    ingester_allow_multiple_replicas_on_same_node: true,
    store_gateway_allow_multiple_replicas_on_same_node: true,
  },
}
```

## Resources

Default scaling of Fire components in the provided Jsonnet is opinionated and based on engineers’ years of experience running it at Grafana Labs.
The default resource requests and limits are also fine-tuned for the provided alerting rules.
For more information, see [Monitor Grafana Fire]({{< relref "../../monitor-grafana-fire/_index.md" >}}).

However, there are use cases where you might want to change the default resource requests, their limits, or both.
For example, if you are just testing Fire and you want to run it on a small (possibly one-node) Kubernetes cluster, and you do not have tens of gigabytes of memory or multiple cores to schedule the components, consider overriding the scaling requirements as follows:

```jsonnet
local k = import 'github.com/grafana/jsonnet-libs/ksonnet-util/kausal.libsonnet',
      deployment = k.apps.v1.deployment,
      statefulSet = k.apps.v1.statefulSet;
local fire = import 'fire/fire.libsonnet';

fire {
  _config+:: {
    // ... configuration values
  },

  compactor_container+: k.util.resourcesRequests('100m', '128Mi'),
  compactor_statefulset+: statefulSet.mixin.spec.withReplicas(1),

  distributor_container+: k.util.resourcesRequests('100m', '128Mi'),
  distributor_deployment+: deployment.mixin.spec.withReplicas(2),

  ingester_container+: k.util.resourcesRequests('100m', '128Mi'),
  ingester_statefulset+: statefulSet.mixin.spec.withReplicas(3),

  querier_container+: k.util.resourcesRequests('100m', '128Mi'),
  querier_deployment+: deployment.mixin.spec.withReplicas(2),

  query_frontend_container+: k.util.resourcesRequests('100m', '128Mi'),
  query_frontend_deployment+: deployment.mixin.spec.withReplicas(2),

  store_gateway_container+: k.util.resourcesRequests('100m', '128Mi'),
  store_gateway_statefulset+: statefulSet.mixin.spec.withReplicas(1),

  local smallMemcached = {
    cpu_requests:: '100m',
    memory_limit_mb:: 64,
    memory_request_overhead_mb:: 8,
    statefulSet+: statefulSet.mixin.spec.withReplicas(1),
  },

  memcached_chunks+: smallMemcached,
  memcached_frontend+: smallMemcached,
  memcached_index_queries+: smallMemcached,
  memcached_metadata+: smallMemcached,
}
```
