---
aliases:
  - /docs/phlare/latest/operators-guide/getting-started/
description: Learn how to get started with Grafana Phlare.
menuTitle: Get started
title: Get started with Grafana Phlare
weight: 10
---

# Get started with Grafana Phlare

There are two different options for getting started with Grafana Phlare:

- The written tutorial provides a series of imperative commands to start a single Phlare process.
- The visual tutorial (in the form of a video) uses `docker-compose` to declaratively deploy multiple Phlare processes.

{{< vimeo 691947043 >}}

<br/>

The written instructions focus on deploying Grafana Phlare as a [monolith]({{< relref "../architecture/deployment-modes/index.md#monolithic-mode" >}}), which is designed for users getting started with the project. For more information about the different ways to deploy Grafana Phlare, refer to [Grafana Phlare deployment modes]({{< relref "../architecture/deployment-modes/index.md" >}}).

## Before you begin

- Verify that you have installed either a [Prometheus server](https://prometheus.io/docs/prometheus/latest/installation/) or the [Grafana Agent](https://grafana.com/docs/grafana-cloud/agent/#installing-the-grafana-agent).
- Verify that you have installed [Docker](https://docs.docker.com/engine/install/).

## Download Grafana Phlare

- Using Docker:

  ```bash
  docker pull grafana/phlare:latest
  ```

- Using a local binary:

  Download the appropriate [release asset](https://github.com/grafana/phlare/releases/latest) for your operating system and architecture and make it executable.

  For Linux with the AMD64 architecture:

  ```bash
  curl -fLo phlare https://github.com/grafana/phlare/releases/latest/download/phlare-linux-amd64
  chmod +x phlare
  ```

## Start Grafana Phlare

To run Grafana Phlare as a monolith and with local filesystem storage, write the following YAML configuration to a file called `demo.yaml`:

<!-- prettier-ignore-start -->
[embedmd]:# (../../../configurations/demo.yaml)
```yaml
# Do not use this configuration in production.
# It is for demonstration purposes only.
multitenancy_enabled: false

blocks_storage:
  backend: filesystem
  bucket_store:
    sync_dir: /tmp/phlare/tsdb-sync
  filesystem:
    dir: /tmp/phlare/data/tsdb
  tsdb:
    dir: /tmp/phlare/tsdb

compactor:
  data_dir: /tmp/phlare/compactor
  sharding_ring:
    kvstore:
      store: memberlist

distributor:
  ring:
    instance_addr: 127.0.0.1
    kvstore:
      store: memberlist

ingester:
  ring:
    instance_addr: 127.0.0.1
    kvstore:
      store: memberlist
    replication_factor: 1

ruler_storage:
  backend: filesystem
  filesystem:
    dir: /tmp/phlare/rules

server:
  http_listen_port: 9009
  log_level: error

store_gateway:
  sharding_ring:
    replication_factor: 1
```
<!-- prettier-ignore-end -->

## Run Grafana Phlare

In a terminal, run one of the following commands:

- Using Docker:

  ```bash
  docker run --rm --name phlare --publish 9009:9009 --volume "$(pwd)"/demo.yaml:/etc/phlare/demo.yaml grafana/phlare:latest --config.file=/etc/phlare/demo.yaml
  ```

- Using a local binary:

  ```bash
  ./phlare --config.file=./demo.yaml
  ```

Grafana Phlare listens on port `9009`.

## Configure Prometheus to write to Grafana Phlare

Add the following YAML snippet to your Prometheus configuration file and restart the Prometheus server:

```yaml
remote_write:
  - url: http://localhost:9009/api/v1/push
```

The configuration for a Prometheus server that scrapes itself and writes those profiles to Grafana Phlare looks similar to this:

```yaml
remote_write:
  - url: http://localhost:9009/api/v1/push

scrape_configs:
  - job_name: prometheus
    honor_labels: true
    static_configs:
      - targets: ["localhost:9090"]
```

## Configure the Grafana Agent to write to Grafana Phlare

Add the following YAML snippet to one of your Agent profiles configurations (`profiles.configs`) in your Agent configuration file and restart the Grafana Agent:

```yaml
remote_write:
  - url: http://localhost:9009/api/v1/push
```

The configuration for an Agent that scrapes itself for profiles and writes those profiles to Grafana Phlare looks similar to this:

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
        - url: http://localhost:9009/api/v1/push
```

## Query data in Grafana

In a new terminal, run a local Grafana server using Docker:

```bash
docker run --rm --name=grafana --network=host grafana/grafana
```

### Add Grafana Phlare as a Prometheus data source

1. In a browser, go to the Grafana server at [http://localhost:3000/datasources](http://localhost:3000/datasources).
1. Sign in using the default username `admin` and password `admin`.
1. Configure a new Prometheus data source to query the local Grafana Phlare server using the following settings:

   | Field | Value                                                                |
   | ----- | -------------------------------------------------------------------- |
   | Name  | Phlare                                                                |
   | URL   | [http://localhost:9009/prometheus](http://localhost:9009/prometheus) |

To add a data source, refer to [Add a data source](https://grafana.com/docs/grafana/latest/datasources/add-a-data-source/).

## Verify success

When you have completed the tasks in this getting started guide, you can query profiles in [Grafana Explore](https://grafana.com/docs/grafana/latest/explore/)
as well as create dashboard panels using the newly configured Grafana Phlare data source.
