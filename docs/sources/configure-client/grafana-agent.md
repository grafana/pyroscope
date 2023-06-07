---
title: "Grafana agent"
menuTitle: "Grafana agent"
description: "Learn more about the Grafana agent"
weight: 10
---

# Grafana agent

The Grafana Agent is responsible for pulling profiles from applications and delivering them
to the Grafana Phlare database.

## Configuring the agent

To be able to pull profiles from applications, your applications needs to expose [pprof endpoints](https://pkg.go.dev/net/http/pprof).

**To configure the agent and see configuration options please see the [agent configuration documentation](/docs/agent/next/flow/reference/components/pyroscope.scrape/).**

### Prerequisites

Before proceeding with the configuration, ensure that you have the following:

- Go application with Pyroscope agent instrumentation (more languages coming soon)
- Docker and Docker Compose installed on your system

### Setup

To set up the Grafana Agent in pull mode, follow these steps:

Clone the repository or navigate to your project directory.

Create the following directory structure for the example files:
```
├── examples
│   └── your-application-example
│       ├── agent
│       │   └── config
│       │       └── config.river
│       ├── docker-compose.yml
│       └── pyroscope
│           └── config.yaml
└── ...
```

Copy the example configuration files from the [Pyroscope example](https://github.com/grafana/pyroscope/tree/main/examples/grafana-agent) to their respective locations in the project directory:

- `examples/your-application-example/agent/config/config.river`
- `examples/your-application-example/docker-compose.yml`
- `examples/your-application-example/pyroscope/config.yaml`

Open the `examples/your-application-example/agent/config/config.river` file and configure the `pyroscope.write` section based on your needs. 

**Ensure that the url property points to the correct Pyroscope instance.**

Open the `examples/your-application-example/docker-compose.yml` file and review the service definitions for pyroscope and agent. Adjust the volumes and ports if needed.

Open the `examples/your-application-example/pyroscope/config.yaml` file and modify the `scrape_configs` section as required. Add the necessary targets for your Go application.

## Running the Grafana Agent

To run the Grafana Agent in pull mode, execute the following steps:

1. Open a terminal or command prompt and navigate to the project directory
2. Start the Grafana Agent and Pyroscope containers using Docker Compose:

`docker-compose -f examples/your-application-example/docker-compose.yml up -d`

3. Wait for the containers to start successfully. You can check their status using the docker ps command.

## Example

Visit the [Pyroscope examples](https://github.com/grafana/pyroscope/tree/main/examples/grafana-agent) to see an example of how to run Pyroscope with the Grafana Agent.