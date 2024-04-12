---
title: "Rust"
menuTitle: "Rust"
description: "Instrumenting Rust applications for continuous profiling."
weight: 60
aliases:
  - /docs/phlare/latest/configure-client/language-sdks/rust
---

# Rust

Optimize your Rust applications with our advanced Rust Profiler. In collaboration with Pyroscope, it offers real-time profiling capabilities, shedding light on the intricacies of your Rust codebase. This integration is invaluable for developers seeking to enhance performance, reduce resource usage, and achieve efficient code execution in Rust applications.

## Before you begin

To capture and analyze profiling data, you need either a hosted Pyroscope OSS server or a hosted [Pryoscope instance with Grafana Cloud Profiles](/products/cloud/profiles-for-continuous-profiling/) (requires a free Grafana Cloud account).

The Pyroscope server can be a local server for development or a remote server for production use.

## Add Rust profiling to your application

Add the `pyroscope` and `pyroscope_pprofrs` crates to your Cargo.toml:

```bash
cargo add pyroscope
cargo add pyroscope_pprofrs
```

## Configure the Rust client

At a minimum, you need to provide the URL of the Pyroscope server and the name
of your application. You also need to configure a profiling backend. For Rust,
you can use [pprof-rs](https://github.com/pyroscope-io/pyroscope-rs/tree/main/pyroscope_backends/pyroscope_pprofrs).

```rust
// Configure profiling backend
let pprof_config = PprofConfig::new().sample_rate(100);
let backend_impl = pprof_backend(pprof_config);

// Configure Pyroscope Agent
let agent = PyroscopeAgent::builder("http://localhost:4040", "myapp")
    .backend(backend_impl)
    .build()?;
```

Users of a secured backend will need to provide authentication details. **Grafana Cloud** uses Basic authentication. Your username is a numeric value which you can get from the "Details Page" for Pyroscope from your stack on grafana.com. On this same page, create a token and use it as the Basic authentication password. The configuration then would look similar to:

```rust
fn  main() ->  Result<()> {
std::env::set_var("RUST_LOG", "debug");
pretty_env_logger::init_timed();
let user = std::env::var("USER").unwrap();
let password = std::env::var("PASSWORD").unwrap();
let url = std::env::var("PYROSCOPE_URL").unwrap();
let samplerate = std::env::var("SAMPLE_RATE").unwrap().to_string().parse().unwrap();
let application_name = "example.basic";

let agent = PyroscopeAgent::builder(url, application_name.to_string())
    .basic_auth(user, password).backend(pprof_backend(PprofConfig::new().sample_rate(samplerate)))
    .tags([("app", "Rust"), ("TagB", "ValueB")].to_vec())
    .build()?;
```

You can start profiling by invoking the following code:

```rust
let agent_running = agent.start().unwrap();
```

The agent can be stopped at any point, and it'll send a last report to the server. The agent can be restarted at a later point.

```rust
let agent_ready = agent.stop().unwrap();
```

It's recommended to shutdown the agent before exiting the application. A last
request to the server might be missed if the agent is not shutdown properly.

```rust
agent_ready.shutdown();
```

## Add profiling labels to Rust applications

Tags can be added or removed after the agent is started. As of 0.5.0, the
Pyroscope Agent supports tagging within threads. Check the [tags](https://github.com/pyroscope-io/pyroscope-rs/blob/main/examples/tags.rs) and [multi-thread](https://github.com/pyroscope-io/pyroscope-rs/blob/main/examples/multi-thread.rs) examples for detailed usage.

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
let agent_ready = running_agent.stop();
```

## Rust client configuration options

The agent accepts additional initial parameters:

- **Backend**: Profiling backend. For Rust, it's [pprof-rs](https://github.com/pyroscope-io/pyroscope-rs/tree/main/pyroscope_backends/pyroscope_pprofrs)
- **Sample Rate**: Sampling Frequency in Hertz. Default is 100.
- **Tags**: Initial tags.

```rust
// Configure Profiling backend
let pprof_config = PprofConfig::new().sample_rate(100);
let pprof_backend = Pprof::new(pprof_config);

// Configure Pyroscope Agent
let agent =
PyroscopeAgent::builder("http://localhost:4040", "myapp")
// Profiling backend
.backend(pprof_backend)
// Sample rate
.sample_rate(100)
// Tags
.tags(vec![("env", "dev")])
// Create the agent
.build()?;
```

## Technical details

- **Backend**: The Pyroscope Agent uses [pprof-rs](https://github.com/tikv/pprof-rs) as a backend. As a result, the [limitations](https://github.com/tikv/pprof-rs#why-not-) for pprof-rs also applies.
As of 0.5.0, the Pyroscope Agent supports tagging within threads. Check the [tags](https://github.com/pyroscope-io/pyroscope-rs/blob/main/examples/tags.rs) and [multi-thread](https://github.com/pyroscope-io/pyroscope-rs/blob/main/examples/multi-thread.rs) examples for usage.
- **Timer**: epoll (for Linux) and kqueue (for macOS) are required for a more precise timer.
- **Shutdown**: The Pyroscope Agent might take some time (usually less than 10 seconds) to shutdown properly and drop its threads. For a proper shutdown, it's recommended that you run the `shutdown` function before dropping the agent.

- **Relevant Links**
  - [Github Repository](https://github.com/pyroscope-io/pyroscope-rs)
  - [Cargo crate](https://crates.io/crates/pyroscope)
  - [Crate documentation](https://docs.rs/pyroscope/latest/pyroscope/index.html)

## Examples

### Usage examples

- [**basic**](https://github.com/pyroscope-io/pyroscope-rs/blob/main/examples/basic.rs): Minimal configuration example.
- [**tags**](https://github.com/pyroscope-io/pyroscope-rs/blob/main/examples/tags.rs): Example using Tags.
- [**async**](https://github.com/pyroscope-io/pyroscope-rs/blob/main/examples/async.rs): Example using Async code with Tokio.
- [**multi-thread**](https://github.com/pyroscope-io/pyroscope-rs/blob/main/examples/multi-thread.rs): Example using multiple threads.
- [**with-logger**](https://github.com/pyroscope-io/pyroscope-rs/blob/main/examples/with-logger.rs): Example with logging to stdout.
- [**error**](https://github.com/pyroscope-io/pyroscope-rs/blob/main/examples/error.rs): Example with an invalid server address.

#### Stand-alone examples

- [**basic**](https://github.com/grafana/pyroscope/tree/main/examples/language-sdk-instrumentation/rust/basic): Simple Rust application that uses the Pyroscope Library.
- [**rideshare**](https://github.com/grafana/pyroscope/tree/main/examples/language-sdk-instrumentation/rust/rideshare): A multi-instances web service running on Docker.
