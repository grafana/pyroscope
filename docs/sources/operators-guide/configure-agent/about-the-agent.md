---
title: "Grafana Phlare about the agent"
menuTitle: "About the agent"
description: "Learn more about the Phlare agent"
weight: 10
---

# Grafana Phlare about the agent

The Grafana Phlare Agent is responsible for pulling profiles from applications and delivering them
to the Grafana Phlare database.

## Configuring the agent

To be able to pull profiles from applications, your applications needs to expose [pprof endpoints](https://pkg.go.dev/net/http/pprof).

For more information about how to instrument your application with pprof endpoints see the [language support]({{< relref "./language-support/">}}) section.

By default the agent will pull profiles every 10s with no timeout using `http` scheme. This can be configured in the `scrape_configs` section:

```yaml
scrape_configs:
  - job_name: 'default'
    scrape_interval: 10s
    scrape_timeout: 0s
    scheme: http
```

If you don't provide an override of `profiling_config` for each `scrape_configs`, the agent will use the following default:

```yaml
profiling_config:
  path_prefix: ''
  pprof_config:
    memory:
      enabled: true
      path: "/debug/pprof/allocs"
      delta: false
    block:
      enabled: true
      path: "/debug/pprof/block"
      delta: false
    goroutine:
      enabled: true
      path: "/debug/pprof/goroutine"
      delta: false
    mutex:
      enabled: true
      path: "/debug/pprof/mutex"
      delta: false
    process_cpu:
      enabled: true
      path: "/debug/pprof/profile"
      delta: true
```

For example if you want to:

- fully disable the `goroutine` profile
- change the `memory` to a different path
- add [`fgprof`](https://github.com/felixge/fgprof) mixed I/O and CPU profiles.

You can override the `profiling_config` as follow:

```yaml
scrape_configs:
  - job_name: 'default'
    scrape_interval: 10s
    profiling_config:
      pprof_config:
        goroutine:
          enabled: false
        memory:
          path: "/debug/pprof/heap"
        fgprof:
          enabled: true
          path: "/debug/fgprof"
          delta: true
```

Finally if you're running pprof endpoints below a path prefix, you can specify it in the `profiling_config`:

```yaml
scrape_configs:
  - job_name: 'default'
    scrape_interval: 10s
    scrape_timeout: 0s
    scheme: http
    profiling_config:
      path_prefix: '/app'
```

For more details about available configuration options, please refer to the [configuration reference]({{<relref "../configure/reference-configuration-parameters/#scrape-configs">}}).

## Running the agent

When running Phlare as [monolith]({{<relref "../architecture/deployment-modes/#monolithic-mode">}}) (`-target=all`), the agent is started automatically within the same process and can scrape profiles.

When running Phlare as [microservices]({{<relref "../architecture/deployment-modes/#microservices-mode">}}), you'll have to start an agent manually as a standalone component and point it to your Phlare cluster.
This can be handy if you want to run the agent on a different location than the Phlare database.

To start a standalone agent, you can use the following command:

```bash
./phlare -target=agent -config.file=/path/to/agent-config.yaml
```

In the future, the agent will be integrated into the [Grafana Agent](/docs/agent/latest/), which will remove the need to run a standalone agent if you're already running the Grafana Agent.
