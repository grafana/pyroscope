# Changelog

## main / unreleased

### Grafana Phlare

## 0.5.1

### Grafana Phlare

* [BUGFIX] Ensure config file parses properly (#536) (#537)

## 0.5.0

### Grafana Phlare

* [CHANGE] Move from kubeval to kubeconform (#534)
* [CHANGE] Bump minimal support version to 1.19 (#531)
* [CHANGE] Update go versions used to 1.19.6 (#533)
* [CHANGE] Update prometheus to have latest relabeling support (#528)
* [FEATURE] Ingester limits (#523)
* [ENHANCEMENT] Add -modules support (#497)
* [ENHANCEMENT] Improve annotations based scraping for helm (#529)
* [ENHANCEMENT] Upgrade linter to work with Go Generics (#524)

## 0.4.0

### Grafana Phlare

* [FEATURE] Add distributor limits (#510)

## 0.3.0

### Grafana Phlare

* [CHANGE] Fix cutting by block size and adjust limits by @simonswine in https://github.com/grafana/phlare/pull/514
* [CHANGE] Increase ingesters default limits by @cyriltovena in https://github.com/grafana/phlare/pull/518
* [FEATURE] Flush profiles to disk per row groups by @simonswine in https://github.com/grafana/phlare/pull/486
* [ENHANCEMENT] Add observability using Loki/Tempo and Grafana Agent by @simonswine in https://github.com/grafana/phlare/pull/495
* [ENHANCEMENT] Add a debug image with phlare running through dlv by @simonswine in https://github.com/grafana/phlare/pull/511
* [BUGFIX] Use the correct API in readyHandler by @hi-rustin in https://github.com/grafana/phlare/pull/516

## 0.2.0

### Grafana Phlare

* [FEATURE] Add query-frontend, query-scheduler and querier worker. (#496)
* [ENHANCEMENT] Add missing flags to expand config file (#492)
* [ENHANCEMENT] Use gotestsum 5d6c5cf
* [BUGFIX] Correctly implement connect health checker service (#491)

## 0.1.2

### Grafana Phlare

* [CHANGE] Add an API go module for external usage. (#466)
* [CHANGE] 27bb8d13 Add a github action to release automatically when tagging the repo. (#482)
* [ENHANCEMENT] c2bfdbce Implements a pprof query API. (#474)
* [ENHANCEMENT] 2f036598 Add query subcommand to profilecli for downloading pprof from phlare  (#475)
* [ENHANCEMENT] 9b645e Add accept/encoding gzip to scrape client (#459)
* [ENHANCEMENT] ac09c628 Add support HTTP service discovery (#453)
* [BUGFIX] Fixes the scrape timeout validation. (#465)
* [BUGFIX] Configure Minio correctly in helm (#459)
* [BUGFIX] Usage stats reporter: fix to remove duplicate if block (#483)

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
