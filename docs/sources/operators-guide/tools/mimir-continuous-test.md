---
title: "Grafana fire-continuous-test"
menuTitle: "Fire-continuous-test"
description: "Use fire-continuous-test to continuously run smoke tests on live Grafana Fire clusters."
weight: 30
---

# Grafana fire-continuous-test

As a developer, you can use the standalone fire-continuous-test tool to run smoke tests on live Grafana Fire clusters.
This tool identifies a class of bugs that could be difficult to spot during development.
Two operating modes are supported:

- As a continuously running deployment in your environment, fire-continuous-test can be used to detect issues on a live Grafana Fire cluster over time.
- As an ad-hoc smoke test tool, fire-continuous-test can be used to validate basic functionality after configuration changes are made to a Grafana Fire cluster.

## Download fire-continuous-test

- Using Docker:

```bash
docker pull "grafana/fire-continuous-test:latest"
```

- Using a local binary:

Download the appropriate [fire-continuous-test binary](https://github.com/grafana/fire/releases/latest) for your operating system and architecture, and make it executable.

For Linux with the AMD64 architecture, execute the following command:

```bash
curl -Lo fire-continuous-test https://github.com/grafana/fire/releases/latest/download/fire-continuous-test-linux-amd64
chmod +x fire-continuous-test
```

## Configure fire-continuous-test

Fire-continuous-test requires the endpoints of the backend Grafana Fire clusters and the authentication for writing and querying testing profiles:

- Set `-tests.write-endpoint` to the base endpoint on the write path. Remove any trailing slash from the URL. The tool appends the specific API path to the URL, for example `/api/v1/push` for the remote-write API.
- Set `-tests.read-endpoint` to the base endpoint on the read path. Remove any trailing slash from the URL. The tool appends the specific API path to the URL, for example `/api/v1/query_range` for the range-query API.
- Set the authentication means to use to write and read profiles in tests. By priority order:
  - `-tests.bearer-token` for bearer token authentication.
  - `-tests.basic-auth-user` and `-tests.basic-auth-password` for a basic authentication.
  - `-tests.tenant-id` to the tenant ID, default to `anonymous`.
- Set `-tests.smoke-test` to run the test once and immediately exit. In this mode, the process exit code is non-zero when the test fails.

> **Note:** You can run `fire-continuous-test -help` to list all available configuration options.

## How it works

Fire-continuous-test periodically runs a suite of tests, writes data to Fire, queries that data back, and checks if the query results match what is expected.
The tool exposes profiles that you can use to alert on test failures, and the tool logs the details about the failed tests.

### Exported profiles

Fire-continuous-test exposes the following Prometheus profiles at the `/profiles` endpoint listening on the port that you configured via the flag `-server.profiles-port`:

```bash
# HELP fire_continuous_test_writes_total Total number of attempted write requests.
# TYPE fire_continuous_test_writes_total counter
fire_continuous_test_writes_total{test="<name>"}
{test="<name>"}

# HELP fire_continuous_test_writes_failed_total Total number of failed write requests.
# TYPE fire_continuous_test_writes_failed_total counter
fire_continuous_test_writes_failed_total{test="<name>",status_code="<code>"}

# HELP fire_continuous_test_queries_total Total number of attempted query requests.
# TYPE fire_continuous_test_queries_total counter
fire_continuous_test_queries_total{test="<name>"}

# HELP fire_continuous_test_queries_failed_total Total number of failed query requests.
# TYPE fire_continuous_test_queries_failed_total counter
fire_continuous_test_queries_failed_total{test="<name>"}

# HELP fire_continuous_test_query_result_checks_total Total number of query results checked for correctness.
# TYPE fire_continuous_test_query_result_checks_total counter
fire_continuous_test_query_result_checks_total{test="<name>"}

# HELP fire_continuous_test_query_result_checks_failed_total Total number of query results failed when checking for correctness.
# TYPE fire_continuous_test_query_result_checks_failed_total counter
fire_continuous_test_query_result_checks_failed_total{test="<name>"}
```

### Alerts

[Grafana Fire alerts]({{< relref "../monitor-grafana-fire/installing-dashboards-and-alerts.md" >}}) include checks on failures that fire-continuous-test tracks.
When running fire-continuous-test, use the provided alerts.
