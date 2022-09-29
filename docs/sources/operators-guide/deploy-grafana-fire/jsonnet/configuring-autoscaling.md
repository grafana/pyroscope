---
aliases:
  - /docs/fire/latest/operators-guide/deploying-grafana-fire/jsonnet/configuring-autoscaling/
description: Learn how to configure Grafana Fire autoscaling when using Jsonnet.
menuTitle: Configuring autoscaling
title: Configuring Grafana Fire autoscaling with Jsonnet
weight: 30
---

# Configuring Grafana Fire autoscaling with Jsonnet

Fire Jsonnet supports autoscaling for the following components:

- [Querier]({{< relref "../../architecture/components/querier.md" >}})

Autoscaling, which is based on Prometheus profiles and [KEDA (Kubernetes-based Event Driven Autoscaler)](https://keda.sh), uses Kubernetes’ Horizontal Pod Autoscaler (HPA).

HPA is not configured directly in Jsonnet but it's created and updated by KEDA.
KEDA is an operator, running in the Kubernetes cluster, which is responsible to simplify the setup of HPA with custom profiles (Prometheus in our case).

## How KEDA works

KEDA is a Kubernetes operator aiming to simplify the wiring between HPA and Prometheus.

Kubernetes HPA, out of the box, is not capable of autoscaling based on profiles scraped by Prometheus, but it allows to configure a custom profiles API server which proxies profiles from a datasource (e.g. Prometheus) to Kubernetes.
Setting up the custom profiles API server for Prometheus in a Kubernetes can be a tedious operation, so KEDA offers an operator to set it up automatically.
KEDA supports proxying profiles for a variety of sources, including Prometheus.

### KEDA in a nutshell

- Runs an operator and an external profiles server.
- The profiles server supports proxying for many metric sources, including Prometheus.
- The operator watches for `ScaledObject` custom resource definition (CRD), defining the minimum and maximum replicas, and scaling trigger profiles of a Deployment or StatefulSet, and then configures the related HPA resource. You don't create the HPA resource in Kubernetes, but the operator creates it for you whenever a `ScaledObject` CRD is created (and keeps it updated for its whole lifecycle).

Refers to [KEDA documentation](https://keda.sh) for more information.

### What happens if KEDA is unhealthy

The autoscaling of deployments is always managed by HPA, which is a native Kubernetes feature.
KEDA, as we use it, never changes the number of replicas of Fire Deployments or StatefulSets.

However, if KEDA is not running successfully, there are consequences for Fire autoscaling too:

- `keda-operator` is down (not critical): changes to `ScaledObject` CRD will not be reflected to the HPA until the operator will get back online. HPA functionality is not affected.
- `keda-operator-profiles-apiserver` is down (critical): HPA is not able to fetch updated profiles and it will stop scaling the deployment until profiles will be back. The deployment (e.g. queriers) will keep working but, in case of any surge of traffic, HPA will not be able to detect it (because of a lack of profiles) and so will not scale up.

The [alert `FireQuerierAutoscalerNotActive`]({{< relref "../../monitor-grafana-fire/_index.md" >}}) fires if HPA is unable to scale the deployment for any reason (e.g. unable to scrape profiles from KEDA profiles API server).

## How Kubernetes HPA works

Refer to Kubernetes [Horizontal Pod Autoscaling](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/) documentation to have a full understanding of how HPA works.

## How to enable autoscaling

The following Jsonnet configuration snippet shows an example of how to enable Fire autoscaling with Jsonnet:

```jsonnet
local fire = import 'fire/fire.libsonnet';

fire {
    _config+:: {
        // Enable queriers autoscaling.
        autoscaling_querier_enabled: true,
        autoscaling_querier_min_replicas: 10,
        autoscaling_querier_max_replicas: 40,
        autoscaling_prometheus_url: 'http://prometheus.default:9090/prometheus',
    }
}
```

> **Note**: KEDA will not be installed by Fire jsonnet. You can follow the [Deploying KEDA](https://keda.sh/docs/latest/deploy/) instructions to install it in your Kubernetes cluster.

## How to disable autoscaling

There are two options to disable autoscaling in a Fire cluster:

1. Set minimum replicas = maximum replicas.
2. Decommission HPA.

### Set minimum replicas = maximum replicas

If KEDA and Kubernetes HPA work correctly but the HPA configuration (metric and threshold) are not giving the expected results (e.g. not scaling up when required), a simple solution to bypass the autoscaling algorithm is to set `autoscaling_querier_min_replicas` and `autoscaling_querier_max_replicas` to the same value.

### Decommission HPA

To fully decommission HPA in a Fire cluster you have to:

1. Set `autoscaling_querier_enabled: false`
2. Manually set the expected number of replicas for the given Fire component

The following example shows how to disable querier autoscaler and configure querier Deployment with 10 replicas:

```jsonnet
local k = import 'github.com/grafana/jsonnet-libs/ksonnet-util/kausal.libsonnet';
local deployment = k.apps.v1.deployment;

fire {
    _config+:: {
        autoscaling_querier_enabled: false,
    },

    querier_deployment+: deployment.mixin.spec.withReplicas(10),
}
```
