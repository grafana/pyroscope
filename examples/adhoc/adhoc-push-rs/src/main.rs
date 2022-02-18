use pyroscope::PyroscopeAgent;
use rand::Rng;

#[inline(never)]
fn is_prime(n: i64) -> bool {
    for i in 2..=n {
        if i * i > n {
            return true;
        }
        if n % i == 0 {
            return false;
        }
    }
    false
}

#[inline(never)]
fn slow(n: i64) -> i64 {
    (1..n).sum()
}

#[inline(never)]
fn fast(n: i64) -> i64 {
    let root = (n as f64).sqrt() as i64;
    (1..=n)
        .step_by(root as usize)
        .map(|a| {
            let b = std::cmp::min(a + root - 1, n);
            (b - a + 1) * (a + b) / 2
        })
        .sum()
}

#[inline(never)]
fn run() {
    let mut rng = rand::thread_rng();
    let base = rng.gen_range(0..10);
    for i in 0..40000000 {
        let n = rng.gen_range(1..10001);
        let _ = if is_prime(base + i) { slow(n) } else { fast(n) };
    }
}

fn main() -> Result<(), Box<dyn std::error::Error>> {
    // No need to modify existing settings,
    // pyroscope will override the server address
    let mut agent =
        PyroscopeAgent::builder("http://pyroscope:4040", "adhoc.example.rust").build()?;
    agent.start();
    run();
    agent.stop();
    Ok(())
}
