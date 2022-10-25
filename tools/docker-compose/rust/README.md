# Continuous Profiling for Rust

## Run backend

```shell
$ RUST_LOG=debug cargo run
```

## Collect profile

```shell
$ pprof -http :6060 "http://localhost:3000/debug/pprof/profile?seconds=1"
```

