# Changelog

## main / unreleased

### Grafana Phlare

## 0.1.1

### Grafana Phlare

* [CHANGE] Upgrade all go version used to 1.19.4 [90ecd84c](https://github.com/grafana/phlare/commit/90ecd84c10d25a833d039a6d2b3fb25d8ab2c4d3)
* [CHANGE] Upgrade base image to latest alpine version 1.16.3 [87493d17](https://github.com/grafana/phlare/commit/87493d17)
* [ENHANCEMENT] Add endpoints for fgprof wall clock profiles to Grafana Phlare [#413](https://github.com/grafana/phlare/pull/413)
* [ENHANCEMENT] Helm/Jsonnet Stricter defaults for `podSecurityContext` [#444](https://github.com/grafana/phlare/pull/444)
* [ENHANCEMENT] Add support for Tencent COS object storage [#437](https://github.com/grafana/phlare/pull/437)
* [ENHANCEMENT] Add CLI flag to print version [#406](https://github.com/grafana/phlare/issues/406)
* [BUGFIX] Fixes bug in middleware instrumentation, that failed HTTP2 requests. [#231](https://github.com/grafana/phlare/issues/231)
* [BUGFIX] Fixes a race in the usage reporter [#386](https://github.com/grafana/phlare/issues/386)
* [BUGFIX] Fixes build arteficts report incorrect version numbers [#391](https://github.com/grafana/phlare/issues/391)
* [BUGFIX] Ensure that a path prefix is correctly appended when scraping profiles [#410](https://github.com/grafana/phlare/issues/410)

## 0.1.0

### Grafana Phlare

Initial release

- **Grafana Phlare is a horizontally-scalable, highly-available, multi-tenant continuous profiling aggregation system** with similar architecture to Grafana Mimir, Grafana Loki, and Grafana Tempo.
- **Easy to get started with guides** covering Helm, Tanka, and docker-compose installations.
- **A fully integrated data source in Grafana** to correlate your continuous profiling data with other observability signals using Grafana Explore and dashboards. The native flame graph panel visualization can also be used by other profiling data sources.
- **Phlare packages an Agent** for pulling profiles directly from your applications like Prometheus. We have also provided detailed documentation about how to profile your application written in **Go, Java/JVM, Python, and Rust**.
