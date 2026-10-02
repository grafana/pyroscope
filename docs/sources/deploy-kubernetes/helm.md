---
description: Learn how to get started with Pyroscope using the Helm chart.
menuTitle: Deploy with Helm
title: Deploy Pyroscope using the Helm chart
weight: 50
---

# Deploy Pyroscope using the Helm chart

The [Helm](https://helm.sh/) chart allows you to configure, install, and upgrade Pyroscope within a Kubernetes cluster.

The chart defaults to the [v2 storage architecture](../reference-pyroscope-v2-architecture/about-pyroscope-v2-architecture/) and installs [Grafana Alloy](https://grafana.com/docs/alloy/latest/) to discover and scrape profile endpoints from annotated pods.

## Before you begin

These instructions are common across any flavor of Kubernetes and assume that you know how to install, configure, and operate a Kubernetes cluster as well as use `kubectl`.

Hardware requirements:

- Option A (single binary): A single Kubernetes node with a minimum of 4 cores and 16GiB RAM
- Option B (microservices): Multiple nodes recommended. The default replica counts deploy more than twenty pods.

Software requirements:

- Kubernetes 1.20 or higher
- The `kubectl` command for your version of Kubernetes
- Helm 3 or higher

Verify that you have:

- Access to the Kubernetes cluster
- Persistent storage is enabled in the Kubernetes cluster, which has a default storage class set up. You can [change the default StorageClass](https://kubernetes.io/docs/tasks/administer-cluster/change-default-storage-class/).
- DNS service works in the Kubernetes cluster

## Install the Helm chart in a custom namespace

Use a custom namespace so that you don't have to overwrite the default namespace later in the procedure.

1. Create a unique Kubernetes namespace, for example `pyroscope-test`:

   ```console
   kubectl create namespace pyroscope-test
   ```

   For more details, refer to the Kubernetes documentation about [Creating a new namespace](https://kubernetes.io/docs/tasks/administer-cluster/namespaces/#creating-a-new-namespace).

1. Set up a Helm repository using the following commands:

   ```console
   helm repo add grafana https://grafana.github.io/helm-charts
   helm repo update
   ```

   {{% admonition type="note" %}}
   The Helm chart at [https://grafana.github.io/helm-charts](https://grafana.github.io/helm-charts) is a publication of the source code at [**grafana/pyroscope**](https://github.com/grafana/pyroscope/tree/main/operations/pyroscope/helm/pyroscope). To list available chart versions, run `helm search repo grafana/pyroscope --versions`. To pin a version, add `--version <CHART_VERSION>` to your `helm install` command.
   {{% /admonition %}}

1. Install Pyroscope using the Helm chart using one of the following options:

   - Option A: Install Pyroscope as a single binary. Use this mode _only when you need one Pyroscope instance_. Multiple instances won't share information with each other.

   ```bash
   helm -n pyroscope-test install pyroscope grafana/pyroscope
   ```

   {{% admonition type="note" %}}
   The output of the command contains the query URLs necessary for the following steps, so for a single-binary setup, it will look like this:

   ```
   [...]
   The in-cluster query URL for the data source in Grafana is:

   http://pyroscope.pyroscope-test.svc.cluster.local.:4040
   [...]
   ```
   {{% /admonition %}}

   {{% admonition type="note" %}}
   Option A uses the filesystem storage backend with no persistent volume by default. This setup suits trying Pyroscope locally, but data is lost when the pod is recreated. For production, configure object storage using `pyroscope.structuredConfig`. Refer to [Configure object storage backend](../configure-server/storage/configure-object-storage-backend/).
   {{% /admonition %}}

   - Option B: Install Pyroscope as multiple microservices. In this mode, as you scale out the number of instances, they share a singular backend for storage and querying. This option enables the bundled MinIO object store for local development and testing.

   ```bash
   helm -n pyroscope-test install pyroscope grafana/pyroscope \
     --set architecture.microservices.enabled=true \
     --set minio.enabled=true
   ```

   {{% admonition type="note" %}}
   The output of the command contains the query URLs necessary for the following steps, so for a microservices setup, it will look like this:

   ```
   [...]
   The in-cluster query URL for the data source in Grafana is:

   http://pyroscope-query-frontend.pyroscope-test.svc.cluster.local.:4040
   [...]
   ```
   {{% /admonition %}}

   {{% admonition type="note" %}}
   Option B deploys many pods using the chart's default replica counts. A single node with 4 cores and 16GiB RAM might not schedule every pod. Use a multi-node cluster for the default install, or reduce replicas with `--set` when testing on one node.
   {{% /admonition %}}

1. Check the statuses of the Pyroscope pods:

   ```bash
   kubectl -n pyroscope-test get pods
   ```

   The results look similar to this when you are in microservices mode:

   ```bash
   kubectl -n pyroscope-test get pods
   NAME                                       READY   STATUS    RESTARTS   AGE
   pyroscope-ad-hoc-profiles-7d75b4f9dc-xwpsw   1/1     Running   0          3m23s
   pyroscope-admin-7c474947c-2p5cc              1/1     Running   0          3m23s
   pyroscope-alloy-0                            1/1     Running   0          3m23s
   pyroscope-compaction-worker-0                1/1     Running   0          5s
   pyroscope-compaction-worker-1                1/1     Running   0          37s
   pyroscope-compaction-worker-2                1/1     Running   0          69s
   pyroscope-distributor-7c474947c-2p5cc        1/1     Running   0          3m23s
   pyroscope-distributor-7c474947c-xbszv        1/1     Running   0          3m23s
   pyroscope-metastore-0                        1/1     Running   0          3m23s
   pyroscope-metastore-1                        1/1     Running   0          3m23s
   pyroscope-metastore-2                        1/1     Running   0          3m23s
   pyroscope-minio-0                            1/1     Running   0          3m23s
   pyroscope-query-backend-66bf58dfcc-89gb8     1/1     Running   0          3m23s
   pyroscope-query-backend-66bf58dfcc-p7lnc     1/1     Running   0          3m23s
   pyroscope-query-backend-66bf58dfcc-zbggm     1/1     Running   0          3m23s
   pyroscope-query-frontend-66bf58dfcc-89gb8    1/1     Running   0          3m23s
   pyroscope-query-frontend-66bf58dfcc-p7lnc    1/1     Running   0          3m23s
   pyroscope-segment-writer-0                   1/1     Running   0          3m23s
   pyroscope-segment-writer-1                   1/1     Running   0          3m23s
   pyroscope-segment-writer-2                   1/1     Running   0          3m23s
   pyroscope-tenant-settings-7d75b4f9dc-xwpsw   1/1     Running   0          3m23s
   ```

   In single-binary mode, you should see `pyroscope-0` and `pyroscope-alloy-0`.

1. Wait until all the pods have a status of `Running` or `Completed`, which might take a few minutes.

## Query profiles in Grafana

1. Install Grafana in the same Kubernetes cluster where you installed Pyroscope.

   ```
   helm upgrade -n pyroscope-test --install grafana grafana/grafana \
     --set image.repository=grafana/grafana \
     --set image.tag=main \
     --set env.GF_PLUGINS_PREINSTALL_SYNC=grafana-pyroscope-app \
     --set env.GF_AUTH_ANONYMOUS_ENABLED=true \
     --set env.GF_AUTH_ANONYMOUS_ORG_ROLE=Admin \
     --set env.GF_DIAGNOSTICS_PROFILING_ENABLED=true \
     --set env.GF_DIAGNOSTICS_PROFILING_ADDR=0.0.0.0 \
     --set env.GF_DIAGNOSTICS_PROFILING_PORT=9094 \
     --set-string 'podAnnotations.profiles\.grafana\.com/cpu\.scrape=true' \
     --set-string 'podAnnotations.profiles\.grafana\.com/cpu\.port=9094' \
     --set-string 'podAnnotations.profiles\.grafana\.com/memory\.scrape=true' \
     --set-string 'podAnnotations.profiles\.grafana\.com/memory\.port=9094' \
     --set-string 'podAnnotations.profiles\.grafana\.com/goroutine\.scrape=true' \
     --set-string 'podAnnotations.profiles\.grafana\.com/goroutine\.port=9094'
   ```

   For details, refer to [Deploy Grafana on Kubernetes](/docs/grafana/latest/setup-grafana/installation/kubernetes/).

1. Port-forward Grafana to `localhost`, by using the `kubectl` command:

   ```bash
   kubectl port-forward -n pyroscope-test service/grafana 3000:80
   ```

1. In a browser, go to the Grafana server at [http://localhost:3000](http://localhost:3000).
1. On the left side, go to **Configuration** > **Data sources**.
1. Configure a Pyroscope data source to query the Pyroscope server.

   Set **Name** to `Pyroscope`. Set **URL** to the in-cluster query URL from the Helm install output:

   - Single binary: `http://pyroscope.pyroscope-test.svc.cluster.local.:4040/`
   - Microservices: `http://pyroscope-query-frontend.pyroscope-test.svc.cluster.local.:4040/`

   To add a data source, refer to [Add a data source](/docs/grafana/latest/datasources/add-a-data-source/).

1. Verify success:

   You should be able to query profiles in [Grafana Explore](/docs/grafana/latest/explore/),
   as well as create dashboard panels by using your newly configured Pyroscope data source.

## Optional: Persistently add data source

The deployment of Grafana has no persistent database, so it will not retain settings like the data source configuration across restarts.

To ensure the data source gets provisioned at start-up, create the following `datasources.yaml` file for your deployment mode.

Single binary:

```yaml
datasources:
  pyroscope.yaml:
   apiVersion: 1
   datasources:
   - name: Pyroscope
     type: grafana-pyroscope-datasource
     uid: pyroscope-test
     url: http://pyroscope.pyroscope-test.svc.cluster.local.:4040/
```

Microservices:

```yaml
datasources:
  pyroscope.yaml:
   apiVersion: 1
   datasources:
   - name: Pyroscope
     type: grafana-pyroscope-datasource
     uid: pyroscope-test
     url: http://pyroscope-query-frontend.pyroscope-test.svc.cluster.local.:4040/
```

Modify the Helm deployment by running:

```bash
   helm upgrade -n pyroscope-test --reuse-values grafana grafana/grafana \
     --values datasources.yaml
```

## Optional: Scrape your own workload's profiles

The Pyroscope chart uses a default configuration that causes Grafana Alloy to scrape pods, provided they have the correct annotations.
This functionality uses [relabel_config](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#relabel_config) and [kubernetes_sd_config](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#kubernetes_sd_config) you might be familiar with from Prometheus or Grafana Alloy configuration.

To get Pyroscope to scrape pods, you must add the following annotations to the Pods:

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

The above example will scrape the `memory`, `cpu` and `goroutine` profiles from the `8080` port of the Pod.

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

By default, the port is discovered using named port `http2` or ending with `-metrics` or `-profiles`.
If you don't have a named port, the scraping target is dropped.

If you don't want to use the port name, then you can use the `profiles.grafana.com/<profile-type>.port` annotation to statically specify the port number.

## Next steps

- If you installed an older v1 microservices deployment and need to move to v2 storage, refer to [Migrate from v1 to v2 storage using Helm](../reference-pyroscope-v2-architecture/migrate-from-v1/).
- To deploy with Jsonnet and Tanka instead, refer to [Deploy Pyroscope with Jsonnet and Tanka](tanka-jsonnet/).
