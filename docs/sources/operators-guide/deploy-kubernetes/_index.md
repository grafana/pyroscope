---
description: Learn how to get started with Grafana Phlare using the Helm chart.
menuTitle: Deploy on Kubernetes
title: Deploy Grafana Phlare using the Helm chart
weight: 15
---

# Getting started with Grafana Phlare using the Helm chart

The [Helm](https://helm.sh/) chart allows you to configure, install, and upgrade Grafana Phlare within a Kubernetes cluster.

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

> **Note:** Although this is not strictly necessary, if you want to access Phlare from outside of the Kubernetes cluster, you will need an ingress. This procedure assumes you have an ingress controller set up.

## Install the Helm chart in a custom namespace

Using a custom namespace solves problems later on because you do not have to overwrite the default namespace.

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

   > **Note:** The Helm chart at [https://grafana.github.io/helm-charts](https://grafana.github.io/helm-charts) is a publication of the source code at [**grafana/phlare**](https://github.com/grafana/phlare/tree/main/operations/helm/charts/phlare-distributed).

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

1. Install Grafana Phlare using the Helm chart:

   ```bash
   helm -n phlare-test install phlare grafana/phlare-distributed -f custom.yaml
   ```

   > **Note:** The output of the command contains the write and read URLs necessary for the following steps.

1. Check the statuses of the Phlare pods:

   ```bash
   kubectl -n phlare-test get pods
   ```

   The results look similar to this:

   ```bash
   kubectl -n phlare-test get pods
   NAME                                            READY   STATUS      RESTARTS   AGE
   phlare-minio-78b59f5569-fhlhs                    1/1     Running     0          2m4s
   phlare-nginx-74f8bff8dc-7kr7z                    1/1     Running     0          2m5s
   phlare-distributed-make-bucket-job-z2hc8         0/1     Completed   0          2m4s
   phlare-overrides-exporter-5fd94b745b-htrdr       1/1     Running     0          2m5s
   phlare-query-frontend-68cbbfbfb5-pt2ng           1/1     Running     0          2m5s
   phlare-ruler-56586c9774-28k7h                    1/1     Running     0          2m5s
   phlare-querier-7894f6c5f9-pj9sp                  1/1     Running     0          2m5s
   phlare-querier-7894f6c5f9-cwjf6                  1/1     Running     0          2m4s
   phlare-alertmanager-0                            1/1     Running     0          2m4s
   phlare-distributor-55745599b5-r26kr              1/1     Running     0          2m4s
   phlare-compactor-0                               1/1     Running     0          2m4s
   phlare-store-gateway-0                           1/1     Running     0          2m4s
   phlare-ingester-1                                1/1     Running     0          2m4s
   phlare-ingester-2                                1/1     Running     0          2m4s
   phlare-ingester-0                                1/1     Running     0          2m4s
   ```

1. Wait until all of the pods have a status of `Running` or `Completed`, which might take a few minutes.

## Configure Prometheus to write to Grafana Phlare

You can either configure Prometheus to write to Grafana Phlare or [configure Grafana Agent to write to Phlare](#configure-grafana-agent-to-write-to-grafana-phlare). Although you can configure both, you do not need to.

Make a choice based on whether or not you already have a Prometheus server set up:

- For an existing Prometheus server:

  1. Add the following YAML snippet to your Prometheus configuration file:

     ```yaml
     remote_write:
       - url: http://<ingress-host>/api/v1/push
     ```

     In this case, your Prometheus server writes profiles to Grafana Phlare, based on what is defined in the existing `scrape_configs` configuration.

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

     In this case, your Prometheus server writes profiles to Grafana Phlare that it scrapes from itself.

  1. Start a Prometheus server by using Docker:

     ```bash
     docker run -p 9090:9090  -v <absolute-path-to>/prometheus.yml:/etc/prometheus/prometheus.yml prom/prometheus
     ```

     > **Note:** On Linux systems, if \<ingress-host\> cannot be resolved by the Prometheus server, use the additional command-line flag `--add-host=<ingress-host>:<kubernetes-cluster-external-address>` to set it up.

## Configure Grafana Agent to write to Grafana Phlare

You can either configure Grafana Agent to write to Grafana Phlare or [configure Prometheus to write to Phlare](#configure-prometheus-to-write-to-grafana-phlare). Although you can configure both, you do not need to.

Make a choice based on whether or not you already have a Grafana Agent set up:

- For an existing Grafana Agent:

  1. Add the following YAML snippet to your Grafana Agent profiles configurations (`profiles.configs`):

     ```yaml
     remote_write:
       - url: http://<ingress-host>/api/v1/push
     ```

     In this case, your Grafana Agent will write profiles to Grafana Phlare, based on what is defined in the existing `profiles.configs.scrape_configs` configuration.

  1. Restart the Grafana Agent.

- For a Grafana Agent that does not exist yet:

  1. Write the following configuration to an `agent.yaml` file:

     ```yaml
     profiles:
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

     In this case, your Grafana Agent writes profiles to Grafana Phlare that it scrapes from itself.

  1. Create an empty directory for the write ahead log (WAL) of the Grafana Agent

  1. Start a Grafana Agent by using Docker:

     ```bash
     docker run -v <absolute-path-to-wal-directory>:/etc/agent/data -v <absolute-path-to>/agent.yaml:/etc/agent/agent.yaml -p 12345:12345 grafana/agent
     ```

     > **Note:** On Linux systems, if \<ingress-host\> cannot be resolved by the Grafana Agent, use the additional command-line flag `--add-host=<ingress-host>:<kubernetes-cluster-external-address>` to set it up.

## Query profiles in Grafana

First install Grafana, and then add Phlare as a Prometheus data source.

1. Start Grafana by using Docker:

   ```bash
   docker run --rm --name=grafana -p 3000:3000 grafana/grafana
   ```

   > **Note:** On Linux systems, if \<ingress-host\> cannot be resolved by Grafana, use the additional command-line flag `--add-host=<ingress-host>:<kubernetes-cluster-external-address>` to set it up.

1. In a browser, go to the Grafana server at [http://localhost:3000](http://localhost:3000).
1. Sign in using the default username `admin` and password `admin`.
1. On the left-hand side, go to **Configuration** > **Data sources**.
1. Configure a new Prometheus data source to query the local Grafana Phlare cluster, by using the following settings:

   | Field | Value                              |
   | ----- | ---------------------------------- |
   | Name  | Phlare                              |
   | URL   | http://\<ingress-host\>/prometheus |

   To add a data source, see [Add a data source](https://grafana.com/docs/grafana/latest/datasources/add-a-data-source/).

1. Verify success:

   You should be able to query profiles in [Grafana Explore](http://localhost:3000/explore),
   as well as create dashboard panels by using your newly configured `Phlare` data source.
   For more information, see [Monitor Grafana Phlare]({{< relref "../../monitor-grafana-phlare" >}}).

## Set up metamonitoring

Grafana Phlare metamonitoring collects profiles or logs, or both,
about Grafana Phlare itself.
In the example that follows, metamonitoring scrapes profiles about
Grafana Phlare itself, and then writes those profiles to the same Grafana Phlare instance.

1. To enable metamonitoring in Grafana Phlare, add the following YAML snippet to your Grafana Phlare `custom.yaml` file:

   ```yaml
   metaMonitoring:
     serviceMonitor:
       enabled: true
     grafanaAgent:
       enabled: true
       installOperator: true
       profiles:
         additionalRemoteWriteConfigs:
           - url: "http://phlare-nginx.phlare-test.svc:80/api/v1/push"
   ```

1. Upgrade Grafana Phlare by using the `helm` command:

   ```bash
   helm -n phlare-test upgrade phlare grafana/phlare-distributed -f custom.yaml
   ```

1. From [Grafana Explore](http://localhost:3000/explore), verify that your profiles are being written to Grafana Phlare, by querying `sum(rate(cortex_ingester_ingested_samples_total[$__rate_interval]))`.

## Query profiles in Grafana that is running within the same Kubernetes cluster

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
1. Configure a new Prometheus data source to query the local Grafana Phlare server, by using the following settings:

   | Field | Value                                           |
   | ----- | ----------------------------------------------- |
   | Name  | Phlare                                           |
   | URL   | http://phlare-nginx.phlare-test.svc:80/prometheus |

   To add a data source, see [Add a data source](https://grafana.com/docs/grafana/latest/datasources/add-a-data-source/).

1. Verify success:

   You should be able to query profiles in [Grafana Explore](https://grafana.com/docs/grafana/latest/explore/),
   as well as create dashboard panels by using your newly configured `Phlare` data source.
