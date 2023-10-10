---
title: "Go (pull mode)"
menuTitle: "Go (pull mode)"
description:  "Instrumenting Golang applications for continuous profiling"
weight: 10
---

# Go (pull mode)

The Grafana Agent is responsible for pulling profiles from applications and delivering them
to the Pyroscope database.

### Prerequisites

To be able to pull profiles from applications, your applications needs to expose [pprof endpoints](https://pkg.go.dev/net/http/pprof).

Before proceeding with the configuration, ensure that you have the following:

1. Install Grafana Agent in [flow mode](/docs/agent/latest/flow/setup/install/)
2. Configure Grafana Agent in flow mode
3. Start the Grafana Agent

## Adding Profiling to the Grafana Agent

This guide presents how to configure the Grafana Agent for scraping performance profiles from a pprof endpoint using `pyroscope.scrape` and then write to a reciever defined in `pyroscope.write`.

**While this documentation will contain a simple example for more configuration options visit the agent documentation for [`pyrosope.scrape`](/docs/agent/latest/flow/reference/components/pyroscope.scrape/) and [`pyroscope.write`](/docs/agent/latest/flow/reference/components/pyroscope.write/#pyroscopewrite).**

## Agent Configuration

In [`/agent/config/config.river`](https://github.com/grafana/pyroscope/blob/main/examples/grafana-agent/agent/config/config.river) file you will need two blocks, `pyroscope.write` and `pyroscope.scrape`:

![Grafana agent go pull diagram](/media/docs/pyroscope/grafana_agent_pull_mode_diagram.png)

1. `pyroscope.write` to configure the receivers to which the profiles are forwarded.
2. `pyroscope.scrape` to establish a pprof scraping job for specific targets. The performance profiles obtained from the scraping process are then passed to the receivers specified in `forward_to`. You can define multiple `pyroscope.scrape` components, each distinguished by unique labels.

Here is the general usage:

```river
pyroscope.write "example" {
  // Send metrics to a locally running Pyroscope instance.
  endpoint {
    url = "http://pyroscope:4040"

    // To send data to Grafana Cloud you'll need to provide username and password.
    // basic_auth {
    //   username = "myuser"
    //   password = "mypassword"
    // }
  }
  external_labels = {
    "env" = "staging",
  }
}

pyroscope.scrape "LABEL" {
  targets    = TARGET_LIST
  forward_to = RECEIVER_LIST
}
```

Here, `LABEL` is a unique identifier for the scraping job, `TARGET_LIST` is a list of target endpoints, and `RECEIVER_LIST` is a list of receivers to which scraped profiles are forwarded.

## Profiling Configuration

The `profiling_config` block inside of `pyroscope.scrape` tailors the profiling settings for the scraping targets. It includes a number of blocks, each corresponding to a specific profile type. These blocks can be enabled or disabled, and the scraping path can be customized. A `delta` option is available to scrape the profile as a delta, adding a seconds query parameter to requests.

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

The following configuration sets up a scraping job in `config.river` that scrapes the application. The profiles obtained are then sent over to the receivers as defined by other components.

### Setup project structure

To set up the Grafana Agent for profiling in pull mode (see [example](https://github.com/grafana/pyroscope/tree/main/examples/grafana-agent)), follow these steps:

Create the following directory structure:

```
├── examples
│   └── your-application-example
│       ├── agent
│       │   └── config
│       │       └── config.river
│       │── docker-compose.yml
└── ...
```

### Setup river config

We will use the following `config.river` file to configure the Grafana Agent to scrape profiles from the application and send them to the Pyroscope server. Be sure to replace the `url` property with the correct Pyroscope instance.

**Note: We have swapped out the standard pprof `block`, `mutex` and `memory` profiles with the more efficient [godeltaprof package](https://github.com/grafana/godeltaprof) which produces `godeltaprof_block`, `godeltaprof_mutex` and `godeltaprof_memory`respectively**.

The reason for using this special package is because godeltaprof is a memory profiler specialized for collecting cumulative profiles (heap, block, mutex) efficiently. It is more efficient because it does the delta/merging before producing pprof data, avoiding extra decompression/parsing/allocations/compression.

To start using godeltaprof in pull mode in a Go application, you need to include godeltaprof module in your app:

```bash
go get github.com/grafana/pyroscope-go/godeltaprof@latest
```

and add it to your imports:

```go
import _ "net/http/pprof"
import _ "github.com/grafana/pyroscope-go/godeltaprof/http/pprof"// add this line as well
```

If you do not have ability to update your code then disable all the `goddeltaprof_X` profiles and enable the corresponding standard `X` profiles.

```river
pyroscope.write "example" {
  endpoint {
    url = "http://pyroscope:4040"
  }
}

pyroscope.scrape "default_settings" {
  targets    = [ { "__address__" = "http://localhost:12345", "service_name" = "example_service" } ]
  forward_to = [pyroscope.write.example.receiver]
  profiling_config {
    profile.goroutine {
      enabled = true
      path = "/debug/pprof/goroutine"
      delta = false
    }
    profile.process_cpu {
      enabled = true
      path = "/debug/pprof/profile"
      delta = true
    }
    profile.godeltaprof_memory {
      enabled = true
      path = "/debug/pprof/delta_heap"
    }
    profile.memory {
      enabled = false
      path = "/debug/pprof/heap"
      delta = false
    }
    profile.godeltaprof_mutex {
      enabled = true
      path = "/debug/pprof/delta_mutex"
    }
    profile.mutex {
      enabled = false
      path = "/debug/pprof/mutex"
      delta = false
    }
    profile.godeltaprof_block {
      enabled = true
      path = "/debug/pprof/delta_block"
    }
    profile.block {
      enabled = false
      path = "/debug/pprof/block"
      delta = false
    }
  }
}

```

By adjusting the configuration as demonstrated above, you can enable the Grafana Agent to suitably scrape and process profile data from your pprof endpoints.

## Resources

Visit the [Pyroscope examples](https://github.com/grafana/pyroscope/tree/main/examples/grafana-agent) to see an example of how to run Pyroscope with the Grafana Agent.
