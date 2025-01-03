---
title: Receive profiles from Pyroscope SDKs
menuTitle: Receive SDK profiles
description: Learn how to configure Grafana Alloy to receive profiles from applications using Pyroscope SDKs.
weight: 10
---

# Receive profiles from Pyroscope SDKs

The `pyroscope.receive_http` component in Alloy receives profiles from applications instrumented with Pyroscope SDKs. This approach provides several benefits:
- Lower latency by sending profiles to a local Alloy instance instead of over internet
- Separation of infrastructure concerns (auth, routing) from application code
- Centralized management of authentication and metadata enrichment

For more information about this component, refer to the [pyroscope.receive_http component](https://grafana.com/docs/alloy/<ALLOY_VERSION>/reference/components/pyroscope/pyroscope.receive_http/) documentation.

{{< admonition type="note" >}}
The `pyroscope.receive_http` component is currently in public preview. To use this component, set the `--stability.level` flag to `public-preview`. For more information about Alloy's run usage, refer to the [run command documentation](https://grafana.com/docs/grafana-cloud/send-data/alloy/reference/cli/run/#the-run-command) documentation.
{{< /admonition >}}

To set up profile receiving, you need to:
1. Configure Alloy components
2. Configure your application's SDK
3. Start Alloy

## Configure Alloy components

The configuration requires at least two components:
- [`pyroscope.receive_http`](https://grafana.com/docs/alloy/<ALLOY_VERSION>/reference/components/pyroscope/pyroscope.receive_http/) to receive profiles via HTTP
- [`pyroscope.write`](https://grafana.com/docs/alloy/<ALLOY_VERSION>/reference/components/pyroscope/pyroscope.write/) to forward profiles to Pyroscope

Here's a basic configuration that sets up a simple profile collection pipeline.
It creates a receiver to collect profiles from your applications and forwards them through a writer component to send them to the Pyroscope backend:

```alloy
// Receives profiles over HTTP
pyroscope.receive_http "default" {
   http {
       listen_address = "0.0.0.0"
       listen_port = 9999
   }
   forward_to = [pyroscope.write.backend.receiver]
}

// Forwards profiles to Pyroscope
pyroscope.write "backend" {
   endpoint {
       url = "http://pyroscope:4040"
   }
}
```

## Configure application SDK
Update your application's SDK configuration to point to Alloy's receive endpoint instead of Pyroscope directly. For example, in Go:
```go
config := pyroscope.Config{
    ApplicationName: "my.service.cpu",
    ServerAddress:   "http://localhost:9999", // Alloy's receive endpoint
}
```
Check your specific language SDK documentation for the exact configuration options.

## Examples

The examples in this section provide samples you can use as a starting point for your own configurations. 

Explore the [example](https://github.com/grafana/pyroscope/tree/main/examples/language-sdk-instrumentation/golang-push/rideshare-alloy) in the Pyroscope GitHub repository to learn how to configure Grafana Alloy to receive profiles from a Golang application instrumented with Pyroscope.

### Basic receiving setup

This example shows a basic setup receiving profiles on port 9090 and forwarding them to a local Pyroscope instance:


```alloy
pyroscope.receive_http "default" {
    http {
        listen_address = "0.0.0.0"
        listen_port = 9090
    }
    forward_to = [pyroscope.write.backend.receiver]
}

pyroscope.write "production" {
    endpoint {
        url = "http://localhost:4040"
    }
}
```

### Authentication

To send profiles to an authenticated Pyroscope endpoint:

```alloy
pyroscope.write "production" {
    endpoint {
        url = "http://pyroscope:4040"
        basic_auth {
            username = env("PYROSCOPE_USERNAME")
            password = env("PYROSCOPE_PASSWORD")
        }
    }
}
```

### Adding external labels
External labels are added to all profiles forwarded through the write component. This is useful for adding infrastructure metadata:
```alloy
pyroscope.receive_http "default" {
    http {
        listen_address = "0.0.0.0"
        listen_port = 9999
    }
    forward_to = [pyroscope.write.backend.receiver]
}

pyroscope.write "backend" {
    endpoint {
        url = "http://pyroscope:4040"
    }
    external_labels = {
        "env"      = "production",
        "region"   = "us-west-1",
        "instance" = env("HOSTNAME"),
        "cluster"  = "main",
    }
}
```

### Multiple destinations
Forward received profiles to multiple destinations - useful for testing or migration scenarios:
```alloy
pyroscope.receive_http "default" {
    http {
        listen_address = "0.0.0.0"
        listen_port = 9999
    }
    forward_to = [pyroscope.write.staging.receiver, pyroscope.write.production.receiver]
}

// Send profiles to staging
pyroscope.write "staging" {
    endpoint {
        url = "http://pyroscope-staging:4040"
    }
    external_labels = {
        "env" = "staging",
    }
}

// Send profiles to production
pyroscope.write "production" {
    endpoint {
        url = "http://pyroscope-production:4041"
    }
    external_labels = {
        "env" = "production",
    }
}
```
{{< admonition type="note" >}}
This configuration will duplicate the received profiles and send a copy to each configured `pyroscope.write` component.
{{< /admonition >}}

Another approach is to configure multiple receivers with multiple destinations:
```alloy
pyroscope.receive_http "staging" {
    http {
        listen_address = "0.0.0.0"
        listen_port = 9998
    }
    forward_to = [pyroscope.write.staging.receiver]
}

pyroscope.receive_http "production" {
    http {
        listen_address = "0.0.0.0"
        listen_port = 9999
    }
    forward_to = [pyroscope.write.production.receiver]
}

// Send profiles to staging
pyroscope.write "staging" {
    endpoint {
        url = "http://pyroscope-staging:4040"
    }
    external_labels = {
        "env" = "staging",
    }
}

// Send profiles to production
pyroscope.write "production" {
    endpoint {
        url = "http://pyroscope-production:4041"
    }
    external_labels = {
        "env" = "production",
    }
}
```

For more information about component configuration options, refer to:

- pyroscope.receive_http [documentation](https://grafana.com/docs/alloy/latest/reference/components/pyroscope/pyroscope.write/)
- pyroscope.write [documentation](https://grafana.com/docs/alloy/latest/reference/components/pyroscope/pyroscope.receive_http/)
