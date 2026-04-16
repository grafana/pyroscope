use pyroscope::pyroscope::PyroscopeAgentBuilder;
use pyroscope::backend::{pprof_backend, BackendConfig, PprofConfig};

fn main() -> Result<(), Box<dyn std::error::Error>> {
    std::env::set_var("RUST_LOG", "debug");
    pretty_env_logger::init_timed();

    let agent = PyroscopeAgentBuilder::new(
        "http://pyroscope:4040",
        "rust-app",
        100,
        "pyroscope-rs",
        env!("CARGO_PKG_VERSION"),
        pprof_backend(PprofConfig::default(), BackendConfig::default()),
    )
    .tags(vec![("Hostname", "pyroscope")])
    .build()?;

    let agent_running = agent.start()?;

    let (add_tag, _remove_tag) = agent_running.tag_wrapper();

    add_tag("Batch".to_string(), "first".to_string())?;

    mutex_lock(2);

    add_tag("Batch".to_string(), "second".to_string())?;

    mutex_lock(5);

    add_tag("Batch".to_string(), "third".to_string())?;

    mutex_lock(12);

    let agent_ready = agent_running.stop()?;
    agent_ready.shutdown();

    Ok(())
}

// Generate useless load
fn mutex_lock(n: u64) {
    let mut _i: u64 = 0;

    let start_time = std::time::Instant::now();
    while start_time.elapsed().as_secs() < (n * 10) {
        _i += 1;
    }
}
