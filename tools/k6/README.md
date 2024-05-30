# Load testing for Pyroscope

This directory contains load test scripts and helpers. Load test scripts are stored in the root of this directory, whilst helper functions are kept in `lib`.

## Quick start

### TL;DR

Use this command to run load tests:

```
K6_READ_TOKEN="token" K6_TENANT_ID="tenant" K6_BASE_URL="url" k6 run tools/k6/reads.js
```

### Triggering a test

In order to run the load tests, you need an API token with a read scope. Once you have this token, you can use this command to kick off load tests with default settings:

```
K6_BASE_URL="url" k6 run tools/k6/reads.js
```

This will run the load test locally against the specified Pyroscope deployment. Alternatively, you can trigger the test to run with k6 cloud executors by running:

```
k6 cloud load.js -e "K6_READ_TOKEN=token K6_TENANT_ID=tenant K6_BASE_URL=url"
```

### Configuration options

By default, the tests are configured to run with one VU for 30 seconds. You can tune this from the commandline by using the `--vus N` and `--duration T` parameters, respectively. See the [k6 docs](https://k6.io/docs/using-k6/k6-options/reference/) for more options.

The `K6_BASE_URL` environment variable is required. It specifies the Pyroscope instance to run the load test against. This is the url that is used to push profiles. For example, to run a test against a local Pyroscope instance, you would do:

```
K6_BASE_URL="http://localhost:4040" k6 run tools/k6/reads.js
```

The `K6_READ_TOKEN` and `K6_TENANT_ID` environment variables are used when running load tests against a Pyroscope deployment that requires authentication.

## Tests

### `read.js`

This will run load tests targeting Pyroscope's read API. It issues the following queries in "last 1 hour" and "last 24 hour" time windows:

- `SelectMergeProfile`
- `/render`
- `SelectMergeStacktraces`
- `LabelNames`
- `Series`
- `/render-diff`
