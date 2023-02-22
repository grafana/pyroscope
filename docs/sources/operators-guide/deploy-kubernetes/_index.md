---
description: Learn how to get started with Grafana Phlare using the Helm chart.
menuTitle: Deploy on Kubernetes
title: Deploy Grafana Phlare using the Helm chart
weight: 15
---

# Deploy Grafana Phlare using the Helm chart

The [Helm](https://helm.sh/) chart allows you to configure, install, and upgrade Grafana Phlare within a Kubernetes cluster.

## Before you begin

The instructions that follow are common across any flavor of Kubernetes and assume that you know how to install, configure, and operate a Kubernetes cluster. And that you know how to use `kubectl`.

> **Caution:** Do not use this getting started procedure in a production environment.

Hardware requirements:

- A single Kubernetes node with a minimum of 4 cores and 16GiB RAM

Software requirements:

- Kubernetes 1.20 or higher
- The `kubectl` command for your version of Kubernetes
- Helm 3 or higher

Verify that you have:

- Access to the Kubernetes cluster
- Persistent storage is enabled in the Kubernetes cluster, which has a default storage class set up. You can [change the default StorageClass](https://kubernetes.io/docs/tasks/administer-cluster/change-default-storage-class/).
- DNS service works in the Kubernetes cluster

## Install the Helm chart in a custom namespace

Use a custom namespace so that you do not have to overwrite the default namespace later in the procedure.

1. Create a unique Kubernetes namespace, for example `phlare-test`:

   ```console
   kubectl create namespace phlare-test
   ```

   For more details, see the Kubernetes documentation about [Creating a new namespace](https://kubernetes.io/docs/tasks/administer-cluster/namespaces/#creating-a-new-namespace).

1. Set up a Helm repository using the following commands:

   ```console
   helm repo add grafana https://grafana.github.io/helm-charts
   helm repo update
   ```

   > **Note:** The Helm chart at [https://grafana.github.io/helm-charts](https://grafana.github.io/helm-charts) is a publication of the source code at [**grafana/phlare**](https://github.com/grafana/phlare/tree/main/operations/phlare/helm/phlare).

1. Install Grafana Phlare using the Helm chart using one of the following options:

   - Option A: Install Grafana Phlare as single binary

   ```bash
   helm -n phlare-test install phlare grafana/phlare
   ```

   - Option B: Install Grafana Phlare as micro-services

   ```bash
   # Gather the default config for micro-services
   curl -LO values-micro-services.yaml https://raw.githubusercontent.com/grafana/phlare/main/operations/phlare/helm/phlare/values-micro-services.yaml
   helm -n phlare-test install phlare grafana/phlare --values values-micro-services.yaml
   ```

   > **Note:** The output of the command contains the query URLs necessary for the following steps, so for a micro-service setup it will look like this:

   ```
   [...]
   The in-cluster query URL is:
   http://phlare-querier.phlare-test.svc.cluster.local.:4100
   [...]
   ```

1. Check the statuses of the Phlare pods:

   ```bash
   kubectl -n phlare-test get pods
   ```

   The results look similar to this when you are in micro-services mode:

   ```bash
   kubectl -n phlare-test get pods
   NAME                                 READY   STATUS    RESTARTS   AGE
   phlare-agent-7d75b4f9dc-xwpsw        1/1     Running   0          3m23s
   phlare-distributor-7c474947c-2p5cc   1/1     Running   0          3m23s
   phlare-distributor-7c474947c-xbszv   1/1     Running   0          3m23s
   phlare-ingester-0                    1/1     Running   0          5s
   phlare-ingester-1                    1/1     Running   0          37s
   phlare-ingester-2                    1/1     Running   0          69s
   phlare-minio-0                       1/1     Running   0          3m23s
   phlare-querier-66bf58dfcc-89gb8      1/1     Running   0          3m23s
   phlare-querier-66bf58dfcc-p7lnc      1/1     Running   0          3m23s
   phlare-querier-66bf58dfcc-zbggm      1/1     Running   0          3m23s
   ```

1. Wait until all of the pods have a status of `Running` or `Completed`, which might take a few minutes.

## Query profiles in Grafana

1. Install Grafana in the same Kubernetes cluster where you installed Phlare.

   ```
   helm upgrade -n phlare-test --install grafana grafana/grafana \
     --set image.repository=grafana/grafana \
     --set image.tag=main \
     --set env.GF_FEATURE_TOGGLES_ENABLE=flameGraph \
     --set env.GF_AUTH_ANONYMOUS_ENABLED=true \
     --set env.GF_AUTH_ANONYMOUS_ORG_ROLE=Admin \
     --set env.GF_DIAGNOSTICS_PROFILING_ENABLED=true \
     --set env.GF_DIAGNOSTICS_PROFILING_ADDR=0.0.0.0 \
     --set env.GF_DIAGNOSTICS_PROFILING_PORT=6060 \
     --set-string 'podAnnotations.phlare\.grafana\.com/scrape=true' \
     --set-string 'podAnnotations.phlare\.grafana\.com/port=6060'
   ```

   For details, see [Deploy Grafana on Kubernetes](/docs/grafana/latest/setup-grafana/installation/kubernetes/).

1. Port-forward Grafana to `localhost`, by using the `kubectl` command:

   ```bash
   kubectl port-forward -n phlare-test service/grafana 3000:80
   ```

1. In a browser, go to the Grafana server at [http://localhost:3000](http://localhost:3000).
1. On the left-hand side, go to **Configuration** > **Data sources**.
1. Configure a new Grafana Phlare data source to query the Grafana Phlare server, by using the following settings:

   | Field | Value                                                        |
   | ----- | ------------------------------------------------------------ |
   | Name  | Phlare                                                       |
   | URL   | `http://phlare-querier.phlare-test.svc.cluster.local.:4100/`   |

   To add a data source, see [Add a data source](/docs/grafana/latest/datasources/add-a-data-source/).

1. Verify success:

   You should be able to query profiles in [Grafana Explore](/docs/grafana/latest/explore/),
   as well as create dashboard panels by using your newly configured Phlare data source.

## Optional: Persistently add data source

The deployment of Grafana has no persistent database, so it will not retain settings like the data source configuration across restarts.

To ensure the data source gets provisioned at start-up, create the following `datasources.yaml` file:

```yaml
datasources:
  phlare.yaml:
   apiVersion: 1
   datasources:
   - name: Phlare
     type: phlare
     uid: phlare-test
     url: http://phlare-querier.phlare-test.svc.cluster.local.:4100/
```

Modify the Helm deployment by running:

```bash
   helm upgrade -n phlare-test --reuse-values grafana grafana/grafana \
     --values datasources.yaml
```

## Optional: Scrape your own workload's profiles

The Phlare chart uses a default configuration that causes its agent to scrape Pods, provided they have the correct annotations.
This functionality uses [relabel_config](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#relabel_config) and [kubernetes_sd_config](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#kubernetes_sd_config) that might be familar from the Prometheus or Grafna Agent config.

In order to get Phlare to scrape pods, you must add the following annotations to the the pods:

```yaml
metadata:
  annotations:
    profiles.grafana.com/memory.scrape: "true"
    profiles.grafana.com/memory.port: "8080"
    profiles.grafana.com/cpu.scrape: "true"
    profiles.grafana.com/cpu.port: "8080"
    profiles.grafana.com/goroutine.scrape: "true"
    profiles.grafana.com/goroutine.port: "8080"
```

The above example will scrape the `memory`, `cpu` and `goroutine` profiles from the `8080` port of the pod.

Each profile type has a set of corresponding annotations which allows customization of scraping per profile type.

```yaml
metadata:
  annotations:
    profiles.grafana.com/<profile-type>.scrape: "true"
    profiles.grafana.com/<profile-type>.port: "<port>"
    profiles.grafana.com/<profile-type>.port_name: "<port-name>"
    profiles.grafana.com/<profile-type>.scheme: "<scheme>"
    profiles.grafana.com/<profile-type>.path: "<profile_path>"
```

The full list of profile types supported by annotations is `cpu`, `memory`, `goroutine`, `block` and `mutex`.

The following table describes the annotations:

| Annotation | Description | Default |
| ---------- | ----------- | ------- |
| `profiles.grafana.com/<profile-type>.scrape` | Whether to scrape the profile type. | `false` |
| `profiles.grafana.com/<profile-type>.port` | The port to scrape the profile type from. | `` |
| `profiles.grafana.com/<profile-type>.port_name` | The port name to scrape the profile type from. | `` |
| `profiles.grafana.com/<profile-type>.scheme` | The scheme to scrape the profile type from. | `http` |
| `profiles.grafana.com/<profile-type>.path` | The path to scrape the profile type from. | default golang path |

By default, the port will be discovered using named port `http2` or ending with `-metrics` or `-profiles`.
This means that if you don't have a named port the scraping target will be dropped.

If you don't want to use the port name then you can use the `profiles.grafana.com/<profile-type>.port` annotation to statically specify the port number.
