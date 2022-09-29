---
aliases:
  - /docs/fire/latest/operators-guide/deploying-grafana-fire/
description: Learn how to deploy Grafana Fire on Kubernetes.
keywords:
  - Fire deployment
  - Fire Kubernetes
menuTitle: Deploy on Kubernetes
title: Deploy Grafana Fire on Kubernetes
weight: 12
---

# Deploy Grafana Fire on Kubernetes

You can use Helm or Tanka to deploy Grafana Fire on Kubernetes.

## Helm

A [fire-distributed](https://github.com/grafana/fire/tree/main/operations/helm/charts/fire-distributed) Helm chart that deploys Grafana Fire in [microservices mode]({{< relref "../architecture/deployment-modes/index.md#microservices-mode" >}}) is available in the [grafana/helm-charts](https://grafana.github.io/helm-charts/) Helm repository.

## Jsonnet and Tanka

A [set of Jsonnet files]({{< relref "./jsonnet/_index.md" >}}) that you can use to deploy Grafana Fire in [microservices mode]({{< relref "../architecture/deployment-modes/index.md#microservices-mode" >}}) using Jsonnet and Tanka.
