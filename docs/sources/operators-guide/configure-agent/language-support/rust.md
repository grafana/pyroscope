---
title: "Rust"
menuTitle: "Rust"
description: ""
weight: 30
---

# Rust

To our knowledge there is currently no Rust crate implementing generic profiling handlers for [pprof] endpoints in Rust applications.

There is though the [pprof-rs] crate implementing CPU profiling using the [pprof] format. It is used for [TiKV] and we are also using an example implemented by this crate.

> **Note:** In order to be able to receive profiles, it was important to build the Rust binaries including debug infos and to link the binaries using glibc.

## Example application

The Rust [example application] has implemented an HTTP handler using the following code snippet. The handler creates a CPU profile for the given time in seconds and returns the [pprof] file with gzip compression.

```rust
fn pprof_handler(request: Request<Body>) -> Response<Body> {
    let mut duration = time::Duration::from_secs(2);
    if let Some(query) = request.uri().query() {
        for (k, v) in form_urlencoded::parse(query.as_bytes()) {
            if k == "seconds" {
                duration = time::Duration::from_secs(v.parse::<u64>().unwrap());
            }
        }
    }

    let guard = pprof::ProfilerGuard::new(1_000_000).unwrap();

    // wait for profile to be completed
    thread::sleep(duration);

    let mut body = Vec::new();
    if let Ok(report) = guard.report().build() {
        let profile = report.pprof().unwrap();
        profile.write_to_vec(&mut body).unwrap();
    }

    // gzip profile
    let mut encoder = Encoder::new(Vec::new()).unwrap();
    io::copy(&mut &body[..], &mut encoder).unwrap();
    let gzip_body = encoder.finish().into_result().unwrap();

    Response::builder()
        .header(CONTENT_LENGTH, gzip_body.len() as u64)
        .header(CONTENT_TYPE, "application/octet-stream")
        .body(Body::from(gzip_body))
        .unwrap()
}
```

To test the handlers you can use the [pprof] CLI tool:

```shell
# Profile the cpu for 5 seconds
pprof -http :3000 "http://127.0.0.1:8081/debug/pprof/profile?seconds=5"
```

[pprof]: https://github.com/google/pprof
[pprof-rs]: https://github.com/tikv/pprof-rs
[TiKV]: https://github.com/tikv/tikv
[example application]: https://github.com/grafana/phlare/tree/main/tools/docker-compose/rust/
