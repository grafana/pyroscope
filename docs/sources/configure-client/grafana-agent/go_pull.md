---
title: Set up Go profiling in pull mode
menuTitle: Set up Go profiling in pull mode
description: Learn how to set up Go profiling in pull mode.
weight: 10
---

# Set up Go profiling in pull mode

In pull mode, the Grafana Agent periodically retrieves profiles from Golang applications, specifically targeting the
`/debug/pprof/*` endpoints.


To set up Golang profiling in pull mode, you need to:

1. Expose pprof endpoints
2. Install Grafana Agent
3. Prepare Grafana Agent configuration file
4. Start Grafana Agent

### Expose pprof endpoints

Ensure your Golang application exposes pprof endpoints.

1. Get `godeltaprof` package

    ```bash
    go get github.com/grafana/pyroscope-go/godeltaprof@latest
    ```

2. Import `net/http/pprof` and `godeltaprof/http/pprof` packages at the start of your application.

    ```go
    import _ "net/http/pprof"
    import _ "github.com/grafana/pyroscope-go/godeltaprof/http/pprof"
    ```

### Install Grafana Agent

[//]: # (TODO&#40;korniltsev&#41; What should go here?)

This procedure uses Grafana Agent to send data to Pyroscope using an example River configuration file for the Grafana Agent in Flow mode.
Make sure you have [Grafana Agent in Flow mode](/docs/agent/latest/flow/setup/install/) installed.

### Prepare Grafana Agent Flow configuration file

In the Grafana Agent Flow configuration file, you need to add at least two blocks: `pyroscope.write`
and `pyroscope.scrape`.

1. Add `pyroscope.write` block.
    ```river
    pyroscope.write "write_job_name" {
            endpoint {
                    url = "http://localhost:4040"
            }
    }
    ```

2. Add `pyroscope.scrape` block.
    ```river
    pyroscope.scrape "scrape_job_name" {
            targets    = [{"__address__" = "localhost:4040", "service_name" = "example_service"}]
            forward_to = [pyroscope.write.write_job_name.receiver]

            profiling_config {
                    profile.process_cpu {
                            enabled = true
                    }

                    profile.godeltaprof_memory {
                            enabled = true
                    }

                    profile.memory { // disable memory, use godeltaprof_memory instead
                            enabled = false
                    }

                    profile.godeltaprof_mutex {
                            enabled = true
                    }

                    profile.mutex { // disable mutex, use godeltaprof_mutex instead
                            enabled = false
                    }

                    profile.godeltaprof_block {
                            enabled = true
                    }

                    profile.block { // disable block, use godeltaprof_block instead
                            enabled = false
                    }

                    profile.goroutine {
                            enabled = true
                    }
            }
    }

    ```
3. Save the changes to the file.
### Start Grafana Agent Flow

1. Start a local Pyroscope instance for testing purposes
    ```bash
    docker run -p 4040:4040 grafana/pyroscope
    ```
2. Start Grafana Agent
    ```bash
    grafana-agent-flow run conifguration.river
    ```
3. Open a browser to http://localhost:4040. The page should list profiles.

## Examples

### Send data to Grafana Cloud

Your Grafana Cloud URL, username, and password can be found on the "Details Page" for Pyroscope from your stack on
grafana.com. On this same page, create a token and use it as the Basic authentication password.

```river
pyroscope.write "write_job_name" {
        endpoint {
                url = "<Grafana Cloud URL>"

                basic_auth {
                        username = "<Grafana Cloud User>"
                        password = "<Grafana Cloud Password>"
                }
        }

}
```

### Discover Kubernetes targets

1. Select all pods
  ```river
  discovery.kubernetes "all_pods" {
          role = "pod"
  }
  ```
2. Drop not running pods, create `namespace`, `pod`, `node` and `container` labels.
  Compose `service_name` label based on `namespace` and `container` labels.
  Select only services matching regex pattern `(ns1/.*)|(ns2/container-.*0)`.
    ```river

    discovery.relabel "specific_pods" {
            targets = discovery.kubernetes.all_pods.targets

            rule {
                    action        = "drop"
                    regex         = "Succeeded|Failed|Completed"
                    source_labels = ["__meta_kubernetes_pod_phase"]
            }

            rule {
                    action        = "replace"
                    source_labels = ["__meta_kubernetes_namespace"]
                    target_label  = "namespace"
            }

            rule {
                    action        = "replace"
                    source_labels = ["__meta_kubernetes_pod_name"]
                    target_label  = "pod"
            }

            rule {
                    action        = "replace"
                    source_labels = ["__meta_kubernetes_node_name"]
                    target_label  = "node"
            }

            rule {
                    action        = "replace"
                    source_labels = ["__meta_kubernetes_pod_container_name"]
                    target_label  = "container"
            }

            rule {
                    action        = "replace"
                    regex         = "(.*)@(.*)"
                    replacement   = "${1}/${2}"
                    separator     = "@"
                    source_labels = ["__meta_kubernetes_namespace", "__meta_kubernetes_pod_container_name"]
                    target_label  = "service_name"
            }

            rule {
                    action        = "keep"
                    regex         = "(ns1/.*)|(ns2/container-.*0)"
                    source_labels = ["service_name"]
            }
    }
    ```

3. Use `discovery.relabel.specific_pods.output` as a target for `pyroscope.scrape` block.

    ```river
        pyroscope.scrape "scrape_job_name" {
                targets    = discovery.relabel.specific_pods.output
                ...
        }
    ```

### Exposing pprof endpoints

If you don't use `http.DefaultServeMux`, you can register `/debug/pprof/*` handlers to your own `http.ServeMux`
```go
var mux *http.ServeMux
mux.Handle("/debug/pprof/", http.DefaultServeMux)
```
Or, if you use gorilla/mux:
```go
var router *mux.Router
router.PathPrefix("/debug/pprof").Handler(http.DefaultServeMux)
```

## References
[Example using grafana-agent](https://github.com/grafana/pyroscope/tree/main/examples/grafana-agent-auto-instrumentation).
[pyroscope.scrape](/docs/agent/latest/flow/reference/components/pyroscope.scrape/)
[pyroscope.write](/docs/agent/latest/flow/reference/components/pyroscope.write/)
[discovery.kubernetes](/docs/agent/latest/flow/reference/components/discovery.kubernetes/)
[discovery.docker](/docs/agent/latest/flow/reference/components/discovery.docker/)
[discovery.relabel](/docs/agent/latest/flow/reference/components/discovery.relabel/)
