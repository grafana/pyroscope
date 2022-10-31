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
- The visual tutorial (in the form of a video) uses [`docker-compose`](https://github.com/grafana/phlare/tree/main/tools/docker-compose) to declaratively deploy Phlare and Grafana.

{{< vimeo todo >}}

<br/>

The written instructions focus on deploying Grafana Phlare as a [monolith]({{< relref "../architecture/deployment-modes/index.md#monolithic-mode" >}}), which is designed for users getting started with the project. For more information about the different ways to deploy Grafana Phlare, refer to [Grafana Phlare deployment modes]({{< relref "../architecture/deployment-modes/index.md" >}}).

## Before you begin

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

```yaml
# Do not use this configuration in production.
# It is for demonstration purposes only.
scrape_configs:
  - job_name: "default"
    scrape_interval: "15s"
    static_configs:
      - targets: ["127.0.0.1:4100"]
```

You can also simply download our [demo configuration](https://raw.githubusercontent.com/grafana/phlare/main/cmd/phlare/phlare.yaml) using:

```bash
curl -fLo demo.yaml https://raw.githubusercontent.com/grafana/phlare/main/cmd/phlare/phlare.yaml
```

## Run Grafana Phlare

In a terminal, run one of the following commands:

- Using Docker:

  ```bash
  docker network create phlare-demo
  docker run --rm --name phlare --network=phlare-demo -p 4100:4100 --volume "$(pwd)"/demo.yaml:/etc/phlare/demo.yaml grafana/phlare:latest --config.file=/etc/phlare/demo.yaml
  ```

- Using a local binary:

  ```bash
  ./phlare --config.file=./demo.yaml
  ```

Grafana Phlare listens on port `4100`. You can now verify that Phlare is ready:

```bash
curl localhost:4100/ready
```

## Configure Grafana Phlare to scrape profiles

By default, Grafana Phlare is configured to scrape itself.
To scrape more profiles, you need to configure the `scrape_configs` section of the [configuration file]({{< relref "../configure/reference-configuration-parameters/index.md#scrape-configs" >}}).

To learn more about language integrations and the Phlare agent, refer to [Grafana Phlare Agent]({{< relref "../configure-agent/_index.md" >}}).

## Query data in Grafana

In a new terminal, run a local Grafana server using Docker:

```bash
docker run --rm --name=grafana -p 3000:3000 -e "GF_FEATURE_TOGGLES_ENABLE=flameGraph" --network=phlare-demo grafana/grafana:main
```

### Add Grafana Phlare data source

1. In a browser, go to the Grafana server at [http://localhost:3000/datasources](http://localhost:3000/datasources).
1. Sign in using the default username `admin` and password `admin`.
1. Configure a new Phlare data source to query the local Grafana Phlare server using the following settings:

   | Field | Value                                                                |
   | ----- | -------------------------------------------------------------------- |
   | Name  | Phlare                                                               |
   | URL   | [http://phlare:4100/](http://phlare:4100/) |

To add a data source, refer to [Add a data source](https://grafana.com/docs/grafana/latest/datasources/add-a-data-source/).

## Verify success

When you have completed the tasks in this getting started guide, you can query profiles in [Grafana Explore](https://grafana.com/docs/grafana/latest/explore/)
as well as create dashboard panels using the newly configured Grafana Phlare data source.
