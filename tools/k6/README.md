# Load testing for Pyroscope

This directory contains load test scripts and helpers. Load test scripts are stored in the `tests` directory, whilst helper functions are kept in `lib`.

## Quick start

### TL;DR

To run a test against a local Pyroscope, run

```bash
./run.sh testname
```

To run a test from k6 cloud, run

```bash
./run.sh -c testname
```

Keep in mind when running from k6 cloud, you will need to have a properly configured `.env` file.

### Configuration options

Each load test can be configured with specific environment variables. Descriptions of these variables and their defaults can be found in the `.env.template` file. To modify a test's behavior, copy `.env.template` to `.env`, then modify the variables how you see fit. Once done, `run.sh` will use the new values to configure the test's behavior.

## Tests

### `read.js`

This will run load tests targeting Pyroscope's read API. It issues the following queries in "last 1 hour" and "last 24 hour" time windows:

- `SelectMergeProfile`
- `/render`
- `SelectMergeStacktraces`
- `LabelNames`
- `Series`
- `/render-diff`
