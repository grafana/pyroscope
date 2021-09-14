#!/usr/bin/env bash

set -euo pipefail

# Decide for how long the benchmark will run
BENCH_RUN_FOR="${BENCH_RUN_FOR:-30m}"

PYROSCOPE_ADDRESS="http://pyroscope:4040"
PYROSCOPE_MAIN_ADDRESS="http://pyroscope_main:4040"
PUSHGATEWAY_ADDRESS="http://pushgateway:9091"
PROMETHEUS_ADDRESS="http://prometheus:9090"
GRAFANA_ADDRESS="http://grafana:3000"

# For more info, check the cli documentation
PYROBENCH_UPLOAD_DEST="${PYROBENCH_UPLOAD_DEST:-/screenshots}"
PYROBENCH_UPLOAD_BUCKET="${PYROBENCH_UPLOAD_BUCKET:-}"
PYROBENCH_UPLOAD_TYPE="${PYROBENCH_UPLOAD_TYPE:-}"

# set a default empty value so that we can always report to docker
AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID:-}"
AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY:-}"

# TODO: added by eduardo
PYROBENCH_PROFILE_WIDTH=40
PYROBENCH_PROFILE_DEPTH=100

export DOCKER_BUILDKIT=1
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"

cd $SCRIPT_DIR

trap 'rc=$?; echo "ERR at line ${LINENO} (rc: $rc)"; composeDown; exit $rc' ERR
trap 'rc=$?; echo "EXIT (rc: $rc)"; composeDown; exit $rc' EXIT

function composeDown() {
  docker-compose down
}

function run() {
  # pull latest image
  docker-compose pull

  # build local code
  docker-compose build

  # Start the docker containers
  docker-compose up -d --force-recreate --remove-orphans

  echo "Generating test load"
  docker exec \
    -e "PYROBENCH_PROFILE_DEPTH=$PYROBENCH_PROFILE_DEPTH" \
    -e "PYROBENCH_PROFILE_WIDTH=$PYROBENCH_PROFILE_WIDTH" \
    pr_client_1 ./pyrobench loadgen \
    --log-level=error \
    --server-address="$PYROSCOPE_ADDRESS" \
    --pushgateway-address="$PUSHGATEWAY_ADDRESS" \
    > /dev/null &
  docker exec \
    -e "PYROBENCH_PROFILE_DEPTH=$PYROBENCH_PROFILE_DEPTH" \
    -e "PYROBENCH_PROFILE_WIDTH=$PYROBENCH_PROFILE_WIDTH" \
    pr_client_1 ./pyrobench loadgen \
    --log-level=error \
    --server-address="$PYROSCOPE_MAIN_ADDRESS"  \
    --pushgateway-address="$PUSHGATEWAY_ADDRESS" \
    > /dev/null &

  echo "Sleeping for $BENCH_RUN_FOR"

  # unix timestamp in ms
  start=$(date +%s%3N)
  sleep "$BENCH_RUN_FOR"
  end=$(date +%s%3N)

  # TODO(eh-am): use docker-compose exec
  docker exec \
    -e "AWS_ACCESS_KEY_ID=$AWS_ACCESS_KEY_ID" \
    -e "AWS_SECRET_ACCESS_KEY=$AWS_SECRET_ACCESS_KEY" \
    -e "PYROBENCH_UPLOAD_TYPE=$PYROBENCH_UPLOAD_TYPE" \
    -e "PYROBENCH_UPLOAD_BUCKET=$PYROBENCH_UPLOAD_BUCKET" \
    -e "PYROBENCH_UPLOAD_DEST=$PYROBENCH_UPLOAD_DEST" \
    pr_client_1 ./pyrobench report image \
    --from="$start" \
    --to="$end" \
    --grafana-address "$GRAFANA_ADDRESS" > "$SCRIPT_DIR/image-report"

  docker exec \
    -e "AWS_ACCESS_KEY_ID=$AWS_ACCESS_KEY_ID" \
    -e "AWS_SECRET_ACCESS_KEY=$AWS_SECRET_ACCESS_KEY" \
    pr_client_1 ./pyrobench report table \
    --prometheus-address="$PROMETHEUS_ADDRESS" \
    --queries-file /report.yaml > "$SCRIPT_DIR/table-report"
}

run
