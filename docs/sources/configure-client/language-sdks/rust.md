---
title: "Rust"
menuTitle: "Rust"
description: "Instrumenting Rust applications for continuous profiling."
weight: 60
aliases:
  - /docs/phlare/latest/configure-client/language-sdks/rust
---

# Rust

Optimize your Rust applications with our advanced Rust Profiler.
In collaboration with Pyroscope, it offers real-time profiling capabilities, shedding light on the intricacies of your Rust codebase.
This integration is invaluable for developers seeking to enhance performance, reduce resource usage, and achieve efficient code execution in Rust applications.

{{< admonition type="note" >}}
Refer to [Available profiling types](https://grafana.com/docs/pyroscope/<PYROSCOPE_VERSION>/configure-client/profile-types/) for a list of profile types supported by Rust.
{{< /admonition >}}

## Before you begin

To capture and analyze profiling data, you need either a hosted Pyroscope OSS server or a hosted [Pyroscope instance with Grafana Cloud Profiles](/products/cloud/profiles/) (requires a free Grafana Cloud account).

The Pyroscope server can be a local server for development or a remote server for production use.

## Add Rust profiling to your application

Add the `pyroscope` crate with the `backend-pprof-rs` feature to your Cargo.toml:

```toml
[dependencies]
pyroscope = { version = "2.0.0", features = ["backend-pprof-rs"] }
```

## Configure the Rust client

At a minimum, you need to provide the URL of the Pyroscope server and the name
of your application. You also need to configure a profiling backend. For Rust,
you can use the [pprof-rs backend](https://github.com/grafana/pyroscope-rs) (enabled via the `backend-pprof-rs` feature).

```rust
use pyroscope::pyroscope::PyroscopeAgentBuilder;
use pyroscope::backend::{pprof_backend, PprofConfig, BackendConfig};

// Configure Pyroscope Agent
let agent = PyroscopeAgentBuilder::new(
    "http://localhost:4040",
    "myapp",
    100, // sample rate in Hz
    "pyroscope-rs",
    env!("CARGO_PKG_VERSION"),
    pprof_backend(PprofConfig::default(), BackendConfig::default()),
)
.build()?;
```

Users of a secured backend will need to provide authentication details. **Grafana Cloud** uses Basic authentication. Your username is a numeric value which you can get from the "Details Page" for Pyroscope from your stack on grafana.com. On this same page, create a token and use it as the Basic authentication password. The configuration then would look similar to:

```rust
use pyroscope::pyroscope::PyroscopeAgentBuilder;
use pyroscope::backend::{pprof_backend, PprofConfig, BackendConfig};

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let user = std::env::var("USER").unwrap();
    let password = std::env::var("PASSWORD").unwrap();
    let url = std::env::var("PYROSCOPE_URL").unwrap();

    let agent = PyroscopeAgentBuilder::new(
        url,
        "example.basic",
        100,
        "pyroscope-rs",
        env!("CARGO_PKG_VERSION"),
        pprof_backend(PprofConfig::default(), BackendConfig::default()),
    )
    .basic_auth(user, password)
    .tags(vec![("app", "Rust"), ("TagB", "ValueB")])
    .build()?;
```

You can start profiling by invoking the following code:

```rust
let agent_running = agent.start().unwrap();
```

The agent can be stopped at any point, and it'll send a last report to the server. The agent can be restarted at a later point.

```rust
let agent_ready = agent_running.stop().unwrap();
```

It's recommended to shut down the agent before exiting the application. A last
request to the server might be missed if the agent is not shutdown properly.

```rust
agent_ready.shutdown();
```

### Locate the URL, user, and password in Grafana Cloud Profiles

[//]: # 'Shared content for URl location in Grafana Cloud Profiles'
[//]: # 'This content is located in /pyroscope/docs/sources/shared/locate-url-pw-user-cloud-profiles.md'

{{< docs/shared source="pyroscope" lookup="locate-url-pw-user-cloud-profiles.md" version="latest" >}}

## Add profiling labels to Rust applications

Tags can be added or removed after the agent is started. The Pyroscope Agent supports tagging within threads.

After the agent is started, the `tag_wrapper` function becomes available.
`tag_wrapper` returns a tuple of functions to add and remove tags to the agent
across thread boundaries. This function is available as long as the agent is
running and can be called multiple times.

```rust
// Start Profiling
let agent_running = agent.start().unwrap();

// Generate Tag Wrapper functions
let (add_tag, remove_tag) = agent_running.tag_wrapper();

// Profiled code (with no tags)

// Add tags to the agent
add_tag("key".to_string(), "value".to_string());

// This portion will be profiled with the specified tag.

// Remove tags from the agent
remove_tag("key".to_string(), "value".to_string());

// Stop the agent
let agent_ready = agent_running.stop()?;
```

## Rust client configuration options

The agent accepts additional initial parameters:

- **Backend**: Profiling backend. For Rust, it's the [pprof-rs backend](https://github.com/grafana/pyroscope-rs) (enabled via the `backend-pprof-rs` feature).
- **Sample Rate**: Sampling frequency in Hertz. Default is 100. Set via `PprofConfig` and the `sample_rate` parameter of `PyroscopeAgentBuilder::new()`.
- **Tags**: Initial tags.

```rust
use pyroscope::pyroscope::PyroscopeAgentBuilder;
use pyroscope::backend::{pprof_backend, PprofConfig, BackendConfig};

// Configure Pyroscope Agent
let agent = PyroscopeAgentBuilder::new(
    "http://localhost:4040",
    "myapp",
    100, // sample rate in Hz
    "pyroscope-rs",
    env!("CARGO_PKG_VERSION"),
    pprof_backend(PprofConfig { sample_rate: 100 }, BackendConfig::default()),
)
.tags(vec![("env", "dev")])
.build()?;
```

## Technical details

- **Backend**: The Pyroscope Agent uses [pprof-rs](https://github.com/tikv/pprof-rs) as a backend. As a result, the [limitations](https://github.com/tikv/pprof-rs#why-not-) for pprof-rs also apply. The Pyroscope Agent supports tagging within threads.
- **Timer**: epoll (for Linux) and kqueue (for macOS) are required for a more precise timer.
- **Shutdown**: The Pyroscope Agent might take some time (usually less than 10 seconds) to shut down properly and drop its threads. For a proper shutdown, it's recommended that you run the `shutdown` function before dropping the agent.

- **Relevant Links**
  - [GitHub Repository](https://github.com/grafana/pyroscope-rs)
  - [Cargo crate](https://crates.io/crates/pyroscope)
  - [Crate documentation](https://docs.rs/pyroscope/latest/pyroscope/index.html)

## Examples

### Usage examples

- [**jemalloc**](https://github.com/grafana/pyroscope-rs/blob/main/examples/jemalloc.rs): Memory profiling with jemalloc backend.

#### Stand-alone examples

- [**basic**](https://github.com/grafana/pyroscope/tree/main/examples/language-sdk-instrumentation/rust/basic): Simple Rust application that uses the Pyroscope Library.
- [**rideshare**](https://github.com/grafana/pyroscope/tree/main/examples/language-sdk-instrumentation/rust/rideshare): A multi-instances web service running on Docker.
