---
aliases:
  - /docs/phlare/latest/operators-guide/getting-started/
  - /docs/phlare/latest/operators-guide/get-started/
  - /docs/phlare/latest/get-started/
description: Learn how to get started with Pyroscope.
menuTitle: Get started
title: Get started with Pyroscope
weight: 250
---

# Get started with Pyroscope

Choose one of the following options to get started with Pyroscope:

- The **written tutorial** below provides a series of imperative commands to start a single Pyroscope process, or [monolith]({{< relref "../reference-pyroscope-architecture/deployment-modes/index.md#monolithic-mode" >}}), which is designed for users getting started with the project.

- You can also use [`docker-compose`](https://github.com/grafana/pyroscope/tree/main/tools/docker-compose) to declaratively deploy Pyroscope and Grafana.

For more information on the different ways to deploy Pyroscope, see [Pyroscope deployment modes]({{< relref "../reference-pyroscope-architecture/deployment-modes/index.md" >}}).

{{< youtube id="XL2yTCPy2e0" >}}

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
      # Download Pyroscope v1.0.0 and unpack it to the current folder
      curl -fL https://github.com/grafana/pyroscope/releases/download/v1.0.0/pyroscope_1.0.0_linux_amd64.tar.gz | tar xvz
      ```

1. Run Pyroscope.

    In a terminal, run one of the following commands:

      - Using Docker:

        ```bash
        docker network create pyroscope-demo
        docker run --rm --name pyroscope --network=pyroscope-demo -p 4040:4040 grafana/pyroscope:latest
        ```

      - Using a local binary:

        ```bash
        ./pyroscope
        ```

1. Verify that Pyroscope is ready. Pyroscope listens on port `4040`.

      ```bash
      curl localhost:4040/ready
      ```

1. Configure Pyroscope to scrape profiles.

    By default, Pyroscope is configured to scrape itself.
    To collect more profiles, you must either instrument your application with an SDK or use Grafana Alloy.

    To learn more about language integrations and the Pyroscope agent, refer to [Pyroscope Agent]({{< relref "../configure-client/_index.md" >}}).

## Add a Pyroscope data source and query data

1. In a terminal, run a local Grafana server using Docker:

    ```bash
    docker run --rm --name=grafana \
      --network=pyroscope-demo \
      -p 3000:3000 \
      -e "GF_INSTALL_PLUGINS=grafana-pyroscope-app"\
      -e "GF_AUTH_ANONYMOUS_ENABLED=true" \
      -e "GF_AUTH_ANONYMOUS_ORG_ROLE=Admin" \
      -e "GF_AUTH_DISABLE_LOGIN_FORM=true" \
      grafana/grafana:main
    ```

1. In a browser, go to the Grafana server at [http://localhost:3000/datasources](http://localhost:3000/datasources).

1. Use the following settings to configure a Pyroscope data source to query the local Pyroscope server:

   | Field | Value                                                                |
   | ----- | -------------------------------------------------------------------- |
   | Name  | Pyroscope                                                            |
   | URL   | [http://pyroscope:4040/](http://pyroscope:4040/) OR [http://host.docker.internal:4040/](http://host.docker.internal:4040/) if using Docker  |

  To learn more about adding data sources, refer to [Add a data source](/docs/grafana/<GRAFANA_VERSION>/datasources/add-a-data-source/).

When you have completed the tasks in this getting started guide, you can query profiles in [Grafana Explore](/docs/grafana/<GRAFANA_VERSION>/explore/)
and create dashboard panels using the newly configured Pyroscope data source. For more information on working with dashboards with Grafana, refer to [Panels and visualizations](/docs/grafana/<GRAFANA_VERSION>/panels-visualizations/) in the Grafana documentation.
