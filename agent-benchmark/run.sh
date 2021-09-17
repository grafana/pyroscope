#!/usr/bin/env bash

set -euo pipefail

# Decide for how long the benchmark will run
BENCH_RUN_FOR="${BENCH_RUN_FOR:-10m}"

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

export DOCKER_BUILDKIT=1
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"


function genUuid() {
  echo $(cat /dev/urandom | tr -dc 'a-z' | fold -w 6 | head -n 1)
  return 0
}
# setup a prefix for docker-compose
# this is important to be able to run
# multiple instances of the samples docker-composes
# (think multiple jobs)
PREFIX="${PREFIX:-}"
if [ -z "$PREFIX" ]; then
  PREFIX="$(genUuid)"
fi

cd "$SCRIPT_DIR"

trap 'rc=$?; echo "ERR at line ${LINENO} (rc: $rc)"; composeDown; exit $rc' ERR
trap 'rc=$?; echo "EXIT (rc: $rc)"; composeDown; exit $rc' EXIT

function composeDown() {
  docker-compose -p "$PREFIX" down
}


function run() {
  docker-compose pull

  docker-compose build

  docker-compose -p "$PREFIX" up -d --force-recreate --remove-orphans

  echo "Simulating traffic"
  docker exec "${PREFIX}_client_1" \
    ./pyrobench hotrod \
    --hotrod-address=http://hotrod_without_pyroscope:8080&
  docker exec "${PREFIX}_client_1" \
    ./pyrobench hotrod \
    --hotrod-address=http://hotrod_with_pyroscope:8080&

  echo "Sleeping for $BENCH_RUN_FOR"
  # unix timestamp in ms
  start=$(date +%s%3N)
  sleep "$BENCH_RUN_FOR"
  end=$(date +%s%3N)

  echo "generating meta report"
  docker exec \
    "${PREFIX}_client_1" ./pyrobench report meta \
    --params "BENCH_RUN_FOR=$BENCH_RUN_FOR" > "$SCRIPT_DIR/meta-report"

  echo "generating image report"
  echo "generating image report"
  docker exec \
    -e "AWS_ACCESS_KEY_ID=$AWS_ACCESS_KEY_ID" \
    -e "AWS_SECRET_ACCESS_KEY=$AWS_SECRET_ACCESS_KEY" \
    -e "PYROBENCH_UPLOAD_TYPE=$PYROBENCH_UPLOAD_TYPE" \
    -e "PYROBENCH_UPLOAD_BUCKET=$PYROBENCH_UPLOAD_BUCKET" \
    -e "PYROBENCH_UPLOAD_DEST=$PYROBENCH_UPLOAD_DEST" \
    "${PREFIX}_client_1" ./pyrobench report image \
    --from="$start" \
    --to="$end" \
    --grafana-address "$GRAFANA_ADDRESS" > "$SCRIPT_DIR/image-report"

  echo "generating table report"
  docker exec \
    -e "AWS_ACCESS_KEY_ID=$AWS_ACCESS_KEY_ID" \
    -e "AWS_SECRET_ACCESS_KEY=$AWS_SECRET_ACCESS_KEY" \
    "${PREFIX}_client_1" ./pyrobench report table \
    --prometheus-address="$PROMETHEUS_ADDRESS" \
    --queries-file /report.yaml > "$SCRIPT_DIR/table-report"
}

run
