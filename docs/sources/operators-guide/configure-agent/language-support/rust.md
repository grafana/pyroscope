---
title: "Rust"
menuTitle: "Rust"
description: "Instrumenting Rust applications for continuous profiling"
weight: 30
---

# Rust

## How to add Rust profiling to your application

Add the `pyroscope` and `pyroscope_pprofrs` crates to your Cargo.toml:

```bash
cargo add pyroscope
cargo add pyroscope_pprofrs
```

### Rust client configuration

At a minimum, you need to provide the URL of the Pyroscope Server and the name
of your application. You also need to configure a profiling backend. For Rust,
you can use [pprof-rs](https://github.com/pyroscope-io/pyroscope-rs/tree/main/pyroscope_backends/pyroscope_pprofrs).

```rust
// Configure profiling backend
let pprof_config = PprofConfig::new().sample_rate(100);
let pprof_backend = Pprof::new(pprof_config);

// Configure Pyroscope Agent
let agent =
PyroscopeAgent::builder("http://localhost:4040", "myapp")
.backend(pprof_backend)
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

## How to add profiling labels to Rust applications

Tags can be added or removed after the agent is started. As of 0.5.0, the
Pyroscope Agent supports tagging within threads. Check the [labels](https://github.com/pyroscope-io/pyroscope-rs/blob/main/examples/tags.rs) and [Multi-Thread](https://github.com/pyroscope-io/pyroscope-rs/blob/main/examples/multi-thread.rs) examples for detailed usage.

After the agent is started (Running state), the `tag_wrapper` function becomes
available. `tag_wrapper` returns a tuple of functions to add and remove tags
to the agent across thread boundaries. This function is available as long as
the agent is running and can be called multiple times.

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

## Technical Details
- **Backend**: The Pyroscope Agent uses [pprof-rs](https://github.com/tikv/pprof-rs) as a backend. As a result, the [limitations](https://github.com/tikv/pprof-rs#why-not-) for pprof-rs also applies.
As of 0.5.0, the Pyroscope Agent supports tagging within threads. Check the [Tags](https://github.com/pyroscope-io/pyroscope-rs/blob/main/examples/tags.rs) and [Multi-Thread](https://github.com/pyroscope-io/pyroscope-rs/blob/main/examples/multi-thread.rs) examples for usage.
- **Timer**: epoll (for Linux) and kqueue (for macOS) are required for a more precise timer.
- **Shutdown**: The Pyroscope Agent might take some time (usually less than 10 seconds) to shutdown properly and drop its threads. For a proper shutdown, it's recommended that you run the `shutdown` function before dropping the Agent.
- **Relevant Links**
  - [Github Repository](https://github.com/pyroscope-io/pyroscope-rs)
  - [Cargo crate](https://crates.io/crates/pyroscope)
  - [Crate documentation](https://docs.rs/pyroscope/latest/pyroscope/index.html)

## Examples

**Usage Examples**

- [**basic**](https://github.com/pyroscope-io/pyroscope-rs/blob/main/examples/basic.rs): Minimal configuration example.
- [**tags**](https://github.com/pyroscope-io/pyroscope-rs/blob/main/examples/tags.rs): Example using Tags.
- [**async**](https://github.com/pyroscope-io/pyroscope-rs/blob/main/examples/async.rs): Example using Async code with Tokio.
- [**multi-thread**](https://github.com/pyroscope-io/pyroscope-rs/blob/main/examples/multi-thread.rs): Example using multiple threads.
- [**with-logger**](https://github.com/pyroscope-io/pyroscope-rs/blob/main/examples/with-logger.rs): Example with logging to stdout.
- [**error**](https://github.com/pyroscope-io/pyroscope-rs/blob/main/examples/error.rs): Example with an invalid server address.

#### Stand-alone Examples

- [**basic**](https://github.com/grafana/pyroscope/tree/main/examples/rust/basic): Simple Rust application that uses the Pyroscope Library.
- [**rideshare**](https://github.com/grafana/pyroscope/tree/main/examples/rust/rideshare): A multi-instances web service running on Docker.
