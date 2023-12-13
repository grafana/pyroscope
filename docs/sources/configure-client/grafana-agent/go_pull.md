---
title: "Go (pull mode)"
menuTitle: "Go (pull mode)"
description: "Set up Go profiling in pull mode"
weight: 10
---

# Go (pull mode)

In pull mode, the Grafana Agent periodically retrieves profiles from Golang applications, specifically targeting the
pprof endpoints.

## Set up Go profiling in pull mode

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

[//]: # (3. Optionally if you don't use `http.DefaultServeMux` you can register `pprof` handlers to your own `http.ServeMux`)

[//]: # (   instance. TODO&#40;korniltsev&#41;: not sure if its worth including, or we should let the users figure it out themselves)

[//]: # ()

[//]: # (    ```go)

[//]: # (    var mux *http.ServeMux)

[//]: # (    mux.Handle&#40;"/debug/pprof/", http.DefaultServeMux&#41;)

[//]: # (    ```)

[//]: # ()

[//]: # (   Or if you use gorilla/mux:)

[//]: # ()

[//]: # (    ```go)

[//]: # (    var router *mux.Router)

[//]: # (    router.PathPrefix&#40;"/debug/pprof"&#41;.Handler&#40;http.DefaultServeMux&#41;)

[//]: # (    ```)

### Install Grafana Agent

[//]: # (TODO&#40;korniltsev&#41; What should go here?)

[flow mode](/docs/agent/latest/flow/setup/install/)

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

[//]: # (    To send data to Grafana Cloud you'll need to provide username, password and URL. You can get them from the "Details Page" for Pyroscope from your stack on grafana.com. On this same page, create a token and use it as the Basic authentication password.)

[//]: # (    ```river)

[//]: # (    pyroscope.write "write_job_name" {)

[//]: # (            endpoint {)

[//]: # (                    url = "<Grafana Cloud URL>" )

[//]: # (            })

[//]: # (            basic_auth {)

[//]: # (                    username = "<Grafana Cloud User>")

[//]: # (                    password = "<Grafana Cloud Password>")

[//]: # (            })

[//]: # (    })

[//]: # (   ```)

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

### Start Grafana Agent Flow

1. Start a local pyroscope instance for testing purposes
    ```bash
    docker run -p 4040:4040 grafana/pyroscope 
    ```
2. Start Grafana Agent
    ```bash
    grafana-agent-flow run conifguration.river
    ```
3. Go to http://localhost:4040 and you should see profiles there.

## Examples

### Send data to Grafana Cloud

Your Grafana Cloud URL, username, and password can be found on the "Details Page" for Pyroscope from your stack on
grafana.com. On this same page, create a token and use it as the Basic authentication password.

```river
pyroscope.write "write_job_name" {
        endpoint {
                url = "<Grafana Cloud URL>"
        }
        basic_auth {
                username = "<Grafana Cloud User>"
                password = "<Grafana Cloud Password>"
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

3. And then use `discovery.relabel.specific_pods.targets` as a target for `pyroscope.scrape` block.

    ```river
        pyroscope.scrape "scrape_job_name" {
                targets    = discovery.relabel.specific_pods.output
                ...
        }
    ```

## References

[pyroscope.scrape](/docs/agent/latest/flow/reference/components/pyroscope.scrape/)
[pyroscope.write](/docs/agent/latest/flow/reference/components/pyroscope.write/)
[discovery.kubernetes](/docs/agent/latest/flow/reference/components/discovery.kubernetes/)
[discovery.docker](/docs/agent/latest/flow/reference/components/discovery.docker/)
[discovery.relabel](/docs/agent/latest/flow/reference/components/discovery.relabel/)
