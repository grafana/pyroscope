---
title: "Go (pull mode)"
menuTitle: "Go (pull mode)"
description:  "Instrumenting Golang applications for continuous profiling"
weight: 10
aliases:
  - /docs/phlare/latest/configure-client/grafana-agent/
---

# Go (pull mode)

The Grafana Agent is responsible for pulling profiles from applications and delivering them
to the Pyroscope database.

### Prerequisites

To be able to pull profiles from applications, your applications needs to expose [pprof endpoints](https://pkg.go.dev/net/http/pprof).

Before proceeding with the configuration, ensure that you have the following:

- Go application with Pyroscope agent instrumentation (more languages coming soon)
- Docker and Docker Compose installed on your system

## Configuring the agent

This guide presents how to configure the Grafana Agent for scraping performance profiles from a pprof endpoint using `pyroscope.scrape`. 

Please note, `pyroscope.scrape` is currently a beta feature. As such, it may undergo changes and updates.

**While this documentation will contain a simple example for more configuration options visit the [agent configuration documentation](/docs/agent/next/flow/reference/components/pyroscope.scrape/).**

### Setup

To set up the Grafana Agent in pull mode, follow these steps:

Clone the Pyroscope repository or navigate to your project directory.

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


## Agent Configuration

Use the `pyroscope.scrape` to establish a pprof scraping job for specific targets. The performance profiles obtained from the scraping process are then passed to the receivers specified in `forward_to`.

You can define multiple `pyroscope.scrape` components, each distinguished by unique labels. 

Here is the general usage:

```plaintext
pyroscope.scrape "LABEL" {
  targets    = TARGET_LIST
  forward_to = RECEIVER_LIST
}
```

Here, `LABEL` is a unique identifier for the scraping job, `TARGET_LIST` is a list of target endpoints, and `RECEIVER_LIST` is a list of receivers to which scraped profiles are forwarded.

## Profiling Configuration

The `profiling_config` block tailors the profiling settings for the scraping targets. It includes a number of blocks, each corresponding to a specific profile type. These blocks can be enabled or disabled, and the scraping path can be customized. A `delta` option is available to scrape the profile as a delta, adding a seconds query parameter to requests.

Below are the blocks available within `profiling_config`:

### `profile.memory`

Collects profiles on memory consumption.

| Argument | Type    | Description                                | Default             | Required |
| -------- | ------- | ------------------------------------------ | ------------------- | -------- |
| enabled  | boolean | Enable this profile type to be scraped.    | true                | no       |
| path     | string  | The path to the profile type on the target. | "/debug/pprof/memory" | no       |
| delta    | boolean | Whether to scrape the profile as a delta.  | false               | no       |

### `profile.block`

Collects profiles on process blocking.

| Argument | Type    | Description                                | Default            | Required |
| -------- | ------- | ------------------------------------------ | ------------------ | -------- |
| enabled  | boolean | Enable this profile type to be scraped.    | true               | no       |
| path     | string  | The path to the profile type on the target. | "/debug/pprof/block" | no       |
| delta    | boolean | Whether to scrape the profile as a delta.  | false              | no       |

### `profile.goroutine`

Collects profiles on the number of goroutines.

| Argument | Type    | Description                                | Default                 | Required |
| -------- | ------- | ------------------------------------------ | ----------------------- | -------- |
| enabled  | boolean | Enable this profile type to be scraped.    | true                    | no       |
| path     | string  | The path to the profile type on the target. | "/debug/pprof/goroutine" | no       |
| delta    | boolean | Whether to scrape the profile as a delta.  | false                   | no       |

### `profile.mutex`

Collects profiles on mutexes.

| Argument | Type    | Description                                | Default             | Required |
| -------- | ------- | ------------------------------------------ | ------------------- | -------- |
| enabled  | boolean | Enable this profile type to be scraped.    | true                | no       |
| path     | string  | The path to the profile type on the target. | "/debug/pprof/mutex" | no       |
| delta    | boolean | Whether to scrape the profile as a delta.  | false               | no       |

### `profile.process_cpu`

Collects profiles on CPU consumption for the process.

| Argument | Type    | Description                                | Default                | Required |
| -------- | ------- | ------------------------------------------ | ---------------------- | -------- |
| enabled  | boolean | Enable this profile type to be scraped.    | true                   | no       |
| path     | string  | The path to the profile type on the target. | "/debug/pprof/profile" | no       |
| delta    | boolean | Whether to scrape the profile as a delta.  | true                   | no       |

### `profile.fgprof`

Collects profiles from an fgprof endpoint.

| Argument | Type    | Description                                | Default           | Required |
| -------- | ------- | ------------------------------------------ | ----------------- | -------- |
| enabled  | boolean | Enable this profile type to be scraped.    | false             | no       |
| path     | string  | The path to the profile type on the target. | "/debug/fgprof"   | no       |
| delta    | boolean | Whether to scrape the profile as a delta.  | true              | no       |

### `profile.godeltaprof_memory`

Collects profiles from godeltaprof memory endpoint. The delta is computed on the target.

| Argument | Type    | Description                                | Default                  | Required |
| -------- | ------- | ------------------------------------------ | ------------------------ | -------- |
| enabled  | boolean | Enable this profile type to be scraped.    | false                    | no       |
| path     | string  | The path to the profile type on the target. | "/debug/pprof/delta_heap" | no       |

### `profile.godeltaprof_mutex`

Collects profiles from godeltaprof mutex endpoint. The delta is computed on the target.

| Argument | Type    | Description                                | Default                   | Required |
| -------- | ------- | ------------------------------------------ | ------------------------- | -------- |
| enabled  | boolean | Enable this profile type to be scraped.    | false                     | no       |
| path     | string  | The path to the profile type on the target. | "/debug/pprof/delta_mutex" | no       |

### `profile.godeltaprof_block`

Collects profiles from godeltaprof block endpoint. The delta is computed on the target.

| Argument | Type    | Description                                | Default                   | Required |
| -------- | ------- | ------------------------------------------ | ------------------------- | -------- |
| enabled  | boolean | Enable this profile type to be scraped.    | false                     | no       |
| path     | string  | The path to the profile type on the target. | "/debug/pprof/delta_block" | no       |

## Example

The following configuration sets up a scraping job that scrapes two local applications (the Agent itself and Pyroscope). The profiles obtained are then sent over to the receivers as defined by other components.

```river
pyroscope.scrape "default_settings" {
  targets    = ["http://localhost:12345"]
  forward_to = ["http://localhost:4040"]
  profiling_config {
    profile.goroutine {
      enabled = true
      path = "/debug/pprof/goroutine"
      delta = false
    }
    profile.mutex {
      enabled = true
      path = "/debug/pprof/mutex"
      delta = false
    }
    profile.process_cpu {
      enabled = true
      path = "/debug/pprof/profile"
      delta = true
    }
    profile.godeltaprof_memory {
      enabled = false
      path = "/debug/pprof/delta_heap"
    }
    profile.memory {
      enabled = true
      path = "/debug/pprof/memory"
      delta = false
    }
    profile.godeltaprof_mutex {
      enabled = false
      path = "/debug/pprof/delta_mutex"
    }
    profile.godeltaprof_block {
      enabled = false
      path = "/debug/pprof/delta_block"
    }
    profile.block {
      enabled = true
      path = "/debug/pprof/block"
      delta = false
    }
  }
}

```

By adjusting the configuration as demonstrated above, you can enable the Grafana Agent to suitably scrape and process profile data from your pprof endpoints.


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