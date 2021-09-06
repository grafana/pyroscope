#!/usr/bin/env bash

set -euo pipefail

set -x

# TODO: remove that awk call
# do that from the cli side

rate_pr=$(docker exec pr_client_1 ./pyrobench promquery \
    'avg(rate(pyroscope_http_request_duration_seconds_count{handler="/ingest", instance="pyroscope:4040"}[5m])) by (instance)' \
    --prometheus-address 'http://prometheus:9090' | awk -F'=>' '{ print $2 }' | xargs)

rate_main=$(docker exec pr_client_1 ./pyrobench promquery \
    'avg(rate(pyroscope_http_request_duration_seconds_count{handler="/ingest", instance="pyroscope_main:4040"}[5m])) by (instance)' \
    --prometheus-address 'http://prometheus:9090' | awk -F'=>' '{ print $2 }' | xargs)


total_pr=$(docker exec pr_client_1 ./pyrobench promquery \
    'pyroscope_http_request_duration_seconds_count{handler="/ingest", instance="pyroscope:4040"}' \
    --prometheus-address 'http://prometheus:9090' | awk -F'=>' '{ print $2 }' | xargs)

total_main=$(docker exec pr_client_1 ./pyrobench promquery \
    'pyroscope_http_request_duration_seconds_count{handler="/ingest", instance="pyroscope_main:4040"}' \
    --prometheus-address 'http://prometheus:9090' | awk -F'=>' '{ print $2 }' | xargs)


# very ugly
cat <<EOF
|                                                                 | pr            | main            |
|-----------------------------------------------------------------|---------------|-----------------|
| rate (rate(pyroscope_http_request_duration_seconds_count[5m])   | \`$rate_pr\`  | \`$rate_main\`  |
| total processed (pyroscope_http_request_duration_seconds_count) | \`$total_pr\` | \`$total_main\` |
EOF
