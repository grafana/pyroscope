---
aliases:
  - /docs/mimir/latest/operators-guide/deploying-grafana-mimir/getting-started-helm-charts/
description: Learn how to get started with Grafana Mimir using the Helm chart.
menuTitle: Getting started using the Helm chart
title: Getting started with Grafana Mimir using the Helm chart
weight: 25
---

# Getting started with Grafana Mimir using the Helm chart

The [Helm](https://helm.sh/) chart allows you to configure, install, and upgrade Grafana Mimir within a Kubernetes cluster.

## Before you begin

The instructions that follow are common across any flavor of Kubernetes. They also assume that you know how to install a Kubernetes cluster, and configure and operate it.

It also assumes that you have an understanding of what the `kubectl` command does.

> **Caution:** Do not use this getting-started procedure in a production environment.

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
- An ingress controller is set up in the Kubernetes cluster, for example [ingress-nginx](https://kubernetes.github.io/ingress-nginx/)

> **Note:** Although this is not strictly necessary, if you want to access Mimir from outside of the Kubernetes cluster, you will need an ingress. This procedure assumes you have an ingress controller set up.

## Install the Helm chart in a custom namespace

Using a custom namespace solves problems later on because you do not have to overwrite the default namespace.

1. Create a unique Kubernetes namespace, for example `mimir-test`:

   ```console
   kubectl create namespace mimir-test
   ```

   For more details, see the Kubernetes documentation about [Creating a new namespace](https://kubernetes.io/docs/tasks/administer-cluster/namespaces/#creating-a-new-namespace).

1. Set up a Helm repository using the following commands:

   ```console
   helm repo add grafana https://grafana.github.io/helm-charts
   helm repo update
   ```

   > **Note:** The Helm chart at [https://grafana.github.io/helm-charts](https://grafana.github.io/helm-charts) is a publication of the source code at [**grafana/mimir**](https://github.com/grafana/mimir/tree/main/operations/helm/charts/mimir-distributed).

1. Configure an ingress:

   a. Create a YAML file of Helm values called `custom.yaml`.

   b. Add the following configuration to the file:

   ```yaml
   nginx:
     ingress:
       enabled: true
       ingressClassName: nginx
       hosts:
         - host: <ingress-host>
           paths:
             - path: /
               pathType: Prefix
       tls:
         # empty, disabled.
   ```

   An ingress enables you to externally access a Kubernetes cluster.
   Replace _`<ingress-host>`_ with a suitable hostname that DNS can resolve
   to the external IP address of the Kubernetes cluster.
   For more information, see [Ingress](https://kubernetes.io/docs/concepts/services-networking/ingress/).

   > **Note:** On Linux systems, and if it is not possible for you set up local DNS resolution, you can use the `--add-host=<ingress-host>:<kubernetes-cluster-external-address>` command-line flag to define the _`<ingress-host>`_ local address for the `docker` commands in the examples that follow.

1. Install Grafana Mimir using the Helm chart:

   ```bash
   helm -n mimir-test install mimir grafana/mimir-distributed -f custom.yaml
   ```

   > **Note:** The output of the command contains the write and read URLs necessary for the following steps.

1. Check the statuses of the Mimir pods:

   ```bash
   kubectl -n mimir-test get pods
   ```

   The results look similar to this:

   ```bash
   kubectl -n mimir-test get pods
   NAME                                            READY   STATUS      RESTARTS   AGE
   mimir-minio-78b59f5569-fhlhs                    1/1     Running     0          2m4s
   mimir-nginx-74f8bff8dc-7kr7z                    1/1     Running     0          2m5s
   mimir-distributed-make-bucket-job-z2hc8         0/1     Completed   0          2m4s
   mimir-overrides-exporter-5fd94b745b-htrdr       1/1     Running     0          2m5s
   mimir-query-frontend-68cbbfbfb5-pt2ng           1/1     Running     0          2m5s
   mimir-ruler-56586c9774-28k7h                    1/1     Running     0          2m5s
   mimir-querier-7894f6c5f9-pj9sp                  1/1     Running     0          2m5s
   mimir-querier-7894f6c5f9-cwjf6                  1/1     Running     0          2m4s
   mimir-alertmanager-0                            1/1     Running     0          2m4s
   mimir-distributor-55745599b5-r26kr              1/1     Running     0          2m4s
   mimir-compactor-0                               1/1     Running     0          2m4s
   mimir-store-gateway-0                           1/1     Running     0          2m4s
   mimir-ingester-1                                1/1     Running     0          2m4s
   mimir-ingester-2                                1/1     Running     0          2m4s
   mimir-ingester-0                                1/1     Running     0          2m4s
   ```

1. Wait until all of the pods have a status of `Running` or `Completed`, which might take a few minutes.

## Configure Prometheus to write to Grafana Mimir

You can either configure Prometheus to write to Grafana Mimir or [configure Grafana Agent to write to Mimir](#configure-grafana-agent-to-write-to-grafana-mimir). Although you can configure both, you do not need to.

Make a choice based on whether or not you already have a Prometheus server set up:

- For an existing Prometheus server:

  1. Add the following YAML snippet to your Prometheus configuration file:

     ```yaml
     remote_write:
       - url: http://<ingress-host>/api/v1/push
     ```

     In this case, your Prometheus server writes metrics to Grafana Mimir, based on what is defined in the existing `scrape_configs` configuration.

  1. Restart the Prometheus server.

- For a Prometheus server that does not exist yet:

  1. Write the following configuration to a `prometheus.yml` file:

     ```yaml
     remote_write:
       - url: http://<ingress-host>/api/v1/push

     scrape_configs:
       - job_name: prometheus
         honor_labels: true
         static_configs:
           - targets: ["localhost:9090"]
     ```

     In this case, your Prometheus server writes metrics to Grafana Mimir that it scrapes from itself.

  1. Start a Prometheus server by using Docker:

     ```bash
     docker run -p 9090:9090  -v <absolute-path-to>/prometheus.yml:/etc/prometheus/prometheus.yml prom/prometheus
     ```

     > **Note:** On Linux systems, if \<ingress-host\> cannot be resolved by the Prometheus server, use the additional command-line flag `--add-host=<ingress-host>:<kubernetes-cluster-external-address>` to set it up.

## Configure Grafana Agent to write to Grafana Mimir

You can either configure Grafana Agent to write to Grafana Mimir or [configure Prometheus to write to Mimir](#configure-prometheus-to-write-to-grafana-mimir). Although you can configure both, you do not need to.

Make a choice based on whether or not you already have a Grafana Agent set up:

- For an existing Grafana Agent:

  1. Add the following YAML snippet to your Grafana Agent metrics configurations (`metrics.configs`):

     ```yaml
     remote_write:
       - url: http://<ingress-host>/api/v1/push
     ```

     In this case, your Grafana Agent will write metrics to Grafana Mimir, based on what is defined in the existing `metrics.configs.scrape_configs` configuration.

  1. Restart the Grafana Agent.

- For a Grafana Agent that does not exist yet:

  1. Write the following configuration to an `agent.yaml` file:

     ```yaml
     metrics:
       wal_directory: /tmp/grafana-agent/wal

       configs:
         - name: agent
           scrape_configs:
             - job_name: agent
               static_configs:
                 - targets: ["127.0.0.1:12345"]
           remote_write:
             - url: http://<ingress-host>/api/v1/push
     ```

     In this case, your Grafana Agent writes metrics to Grafana Mimir that it scrapes from itself.

  1. Create an empty directory for the write ahead log (WAL) of the Grafana Agent

  1. Start a Grafana Agent by using Docker:

     ```bash
     docker run -v <absolute-path-to-wal-directory>:/etc/agent/data -v <absolute-path-to>/agent.yaml:/etc/agent/agent.yaml -p 12345:12345 grafana/agent
     ```

     > **Note:** On Linux systems, if \<ingress-host\> cannot be resolved by the Grafana Agent, use the additional command-line flag `--add-host=<ingress-host>:<kubernetes-cluster-external-address>` to set it up.

## Query metrics in Grafana

First install Grafana, and then add Mimir as a Prometheus data source.

1. Start Grafana by using Docker:

   ```bash
   docker run --rm --name=grafana -p 3000:3000 grafana/grafana
   ```

   > **Note:** On Linux systems, if \<ingress-host\> cannot be resolved by Grafana, use the additional command-line flag `--add-host=<ingress-host>:<kubernetes-cluster-external-address>` to set it up.

1. In a browser, go to the Grafana server at [http://localhost:3000](http://localhost:3000).
1. Sign in using the default username `admin` and password `admin`.
1. On the left-hand side, go to **Configuration** > **Data sources**.
1. Configure a new Prometheus data source to query the local Grafana Mimir cluster, by using the following settings:

   | Field | Value                              |
   | ----- | ---------------------------------- |
   | Name  | Mimir                              |
   | URL   | http://\<ingress-host\>/prometheus |

   To add a data source, see [Add a data source](https://grafana.com/docs/grafana/latest/datasources/add-a-data-source/).

1. Verify success:

   You should be able to query metrics in [Grafana Explore](http://localhost:3000/explore),
   as well as create dashboard panels by using your newly configured `Mimir` data source.
   For more information, see [Monitor Grafana Mimir]({{< relref "../../monitor-grafana-mimir" >}}).

## Set up metamonitoring

Grafana Mimir metamonitoring collects metrics or logs, or both,
about Grafana Mimir itself.
In the example that follows, metamonitoring scrapes metrics about
Grafana Mimir itself, and then writes those metrics to the same Grafana Mimir instance.

1. To enable metamonitoring in Grafana Mimir, add the following YAML snippet to your Grafana Mimir `custom.yaml` file:

   ```yaml
   metaMonitoring:
     serviceMonitor:
       enabled: true
     grafanaAgent:
       enabled: true
       installOperator: true
       metrics:
         additionalRemoteWriteConfigs:
           - url: "http://mimir-nginx.mimir-test.svc:80/api/v1/push"
   ```

1. Upgrade Grafana Mimir by using the `helm` command:

   ```bash
   helm -n mimir-test upgrade mimir grafana/mimir-distributed -f custom.yaml
   ```

1. From [Grafana Explore](http://localhost:3000/explore), verify that your metrics are being written to Grafana Mimir, by querying `sum(rate(cortex_ingester_ingested_samples_total[$__rate_interval]))`.

## Query metrics in Grafana that is running within the same Kubernetes cluster

1. Install Grafana in the same Kubernetes cluster.

   For details, see [Deploy Grafana on Kubernetes](https://grafana.com/docs/grafana/latest/setup-grafana/installation/kubernetes/).

1. Stop the Grafana instance that is running in the Docker container, to allow for port-forwarding.

1. Port-forward Grafana to `localhost`, by using the `kubectl` command:

   ```bash
   kubectl port-forward service/grafana 3000:3000
   ```

1. In a browser, go to the Grafana server at [http://localhost:3000](http://localhost:3000).
1. Sign in using the default username `admin` and password `admin`.
1. On the left-hand side, go to **Configuration** > **Data sources**.
1. Configure a new Prometheus data source to query the local Grafana Mimir server, by using the following settings:

   | Field | Value                                           |
   | ----- | ----------------------------------------------- |
   | Name  | Mimir                                           |
   | URL   | http://mimir-nginx.mimir-test.svc:80/prometheus |

   To add a data source, see [Add a data source](https://grafana.com/docs/grafana/latest/datasources/add-a-data-source/).

1. Verify success:

   You should be able to query metrics in [Grafana Explore](https://grafana.com/docs/grafana/latest/explore/),
   as well as create dashboard panels by using your newly configured `Mimir` data source.
