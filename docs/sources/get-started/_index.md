---
aliases:
  - /docs/phlare/latest/operators-guide/getting-started/
  - /docs/phlare/latest/operators-guide/get-started/
  - /docs/phlare/latest/get-started/
description: Learn how to get started with Pyroscope.
menuTitle: Get started
title: Get started with Pyroscope
weight: 20
---

# Get started with Pyroscope

Choose one of the following options to get started with Pyroscope:

- The **written tutorial** below provides a series of imperative commands to start a single Pyroscope process, or [monolith]({{< relref "../reference-pyroscope-architecture/deployment-modes/index.md#monolithic-mode" >}}), which is designed for users getting started with the project.

- The following **video tutorial** uses [`docker-compose`](https://github.com/grafana/pyroscope/tree/main/tools/docker-compose) to declaratively deploy Pyroscope and Grafana.

  {{< vimeo 766316030 >}}

For more information on the different ways to deploy Pyroscope, see [Pyroscope deployment modes]({{< relref "../reference-pyroscope-architecture/deployment-modes/index.md" >}}).

## Before you begin

Verify that you have installed [Docker](https://docs.docker.com/engine/install/).

## Download and configure Pyroscope

1. Download Pyroscope.

    You can use Docker or download a binary to install Pyroscope.

    - To install with Docker, run the following command:

      ```bash
      docker pull grafana/pyroscope:latest
        ```

    - To use a local binary:

      Download the appropriate [release asset](https://github.com/grafana/pyroscope/releases/latest) for your operating system and architecture and make it executable.

      For example, for Linux with the AMD64 architecture:

      ```bash
      # Download Pyroscope v0.1.1 and unpack it to the current folder
      curl -fL https://github.com/grafana/pyroscope/releases/download/v0.1.1/pyroscope.1.1_linux_amd64.tar.gz | tar xvz
      ```

1. Start Pyroscope.

    To run Pyroscope as a monolith and with local filesystem storage, you can create your own file, or use a demo configuration file.

    - To create your own file, write the following YAML configuration to a file called `demo.yaml`:

      ```yaml
      # Do not use this configuration in production.
      # It is for demonstration purposes only.
      scrape_configs:
        - job_name: "default"
          scrape_interval: "15s"
          static_configs:
            - targets: ["127.0.0.1:4040"]
              labels:
                service_name: "service-demo"
      ```

    - To use a demo file, download our [demo configuration](https://raw.githubusercontent.com/grafana/pyroscope/main/cmd/pyroscope/pyroscope.yaml):

      ```bash
      curl -fLo demo.yaml https://raw.githubusercontent.com/grafana/pyroscope/main/cmd/pyroscope/pyroscope.yaml
      ```

1. Run Pyroscope.

    In a terminal, run one of the following commands:

      - Using Docker:

        ```bash
        docker network create pyroscope-demo
        docker run --rm --name pyroscope --network=pyroscope-demo -p 4040:4040 --volume "$(pwd)"/demo.yaml:/etc/pyroscope/demo.yaml grafana/pyroscope:latest --config.file=/etc/pyroscope/demo.yaml
        ```

      - Using a local binary:

        ```bash
        ./pyroscope --config.file=./demo.yaml
        ```

1. Verify that Pyroscope is ready. Pyroscope listens on port `4040`.

      ```bash
      curl localhost:4040/ready
      ```

1. Configure Pyroscope to scrape profiles.

    By default, Pyroscope is configured to scrape itself.
    To scrape more profiles, you must configure the `scrape_configs` section of the [configuration file]({{< relref "../configure-server/reference-configuration-parameters/index.md#scrape-configs" >}}).

    To learn more about language integrations and the Pyroscope agent, refer to [Pyroscope Agent]({{< relref "../configure-client/_index.md" >}}).

## Add a Pyroscope data source and query data

1. In a new terminal, run a local Grafana server using Docker:

    ```bash
    docker run --rm --name=grafana -p 3000:3000 -e "GF_FEATURE_TOGGLES_ENABLE=flameGraph" --network=pyroscope-demo grafana/grafana:main
    ```

1. In a browser, go to the Grafana server at [http://localhost:3000/datasources](http://localhost:3000/datasources).

1. Sign in using the default username `admin` and password `admin`.

1. Use the following settings to configure a Pyroscope data source to query the local Pyroscope server:

   | Field | Value                                                                |
   | ----- | -------------------------------------------------------------------- |
   | Name  | Pyroscope                                                            |
   | URL   | [http://pyroscope:4040/](http://pyroscope:4040/)                     |

  To learn more about adding data sources, see [Add a data source](/docs/grafana/latest/datasources/add-a-data-source/).

When you have completed the tasks in this getting started guide, you can query profiles in [Grafana Explore](/docs/grafana/latest/explore/)
and create dashboard panels using the newly configured Pyroscope data source. For more information on working with dashboards with Grafana, see [Panels and visualizations](/docs/grafana/latest/panels-visualizations/) in the Grafana documentation.
