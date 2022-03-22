use pyroscope::PyroscopeAgent;

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let mut agent = PyroscopeAgent::builder("http://localhost:4040", "rust-app")
        .sample_rate(100)
        .tags(&[("Hostname", "pyroscope")])
        .build()?;

    // Start Agent
    agent.start();

    // Add tag
    agent.add_tags(&[("Batch", "first")])?;

    // Do some work for first batch.
    mutex_lock(2);

    // Change Tag
    agent.add_tags(&[("Batch", "second")])?;

    // Do some work for second batch.
    mutex_lock(5);

    // Change Tag
    agent.add_tags(&[("Batch", "third")])?;

    // Do some work for third batch.
    mutex_lock(12);

    agent.remove_tags(&["Batch"])?;

    // Stop Agent
    agent.stop();
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
