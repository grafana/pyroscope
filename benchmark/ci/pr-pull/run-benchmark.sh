#!/usr/bin/env bash

set -euo pipefail

# Decide for how long the benchmark will run
BENCH_RUN_FOR="${BENCH_RUN_FOR:-10m}"

PYROSCOPE_ADDRESS="http://pyroscope:4040"
PYROSCOPE_MAIN_ADDRESS="http://pyroscope_main:4040"
PUSHGATEWAY_ADDRESS="http://pushgateway:9091"
PROMETHEUS_ADDRESS="http://prometheus:9090"
GRAFANA_ADDRESS="http://grafana:3000"

# since we gonna report this values in the meta table
# let's define them explicitly here
PYROBENCH_RAND_SEED="${PYROBENCH_RAND_SEED:-2306912}"
PYROBENCH_PROFILE_WIDTH="${PYROBENCH_PROFILE_WIDTH:-20}"
PYROBENCH_PROFILE_DEPTH="${PYROBENCH_PROFILE_DEPTH:-20}"
PYROBENCH_PROFILE_SYMBOL_LENGTH="${PYROBENCH_PROFILE_SYMBOL_LENGTH:-30}"
PYROBENCH_APPS="${PYROBENCH_APPS:-20}"
PYROBENCH_CLIENTS="${PYROBENCH_CLIENTS:-20}"
PYROBENCH_REQUESTS="${PYROBENCH_REQUESTS:-10000}"
PYROBENCH_PULL_CLIENTS="${PYROBENCH_PULL_CLIENTS:-40}"


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

cd $SCRIPT_DIR

trap 'rc=$?; echo "ERR at line ${LINENO} (rc: $rc)"; composeDown; exit $rc' ERR
trap 'rc=$?; echo "EXIT (rc: $rc)"; composeDown; exit $rc' EXIT

function composeDown() {
  docker-compose -p "$PREFIX" down
}

function run() {

  # pull latest image
  docker-compose pull

  # build local code
  docker-compose build

  # prepare config
  cat ../../pull/server.yml.example | grep -B 1000 'static-configs:' > server.yml

  for ((x=0 ; x<$PYROBENCH_PULL_CLIENTS ; x++)); do
    echo "      - application: pull-target" >> server.yml
    echo "        targets:"                 >> server.yml
    echo "          - pull-target:4042"     >> server.yml
    echo "        labels:"                  >> server.yml
    echo "          pod: pod-$x"            >> server.yml
  done

  # Start the docker containers
  docker-compose -p "$PREFIX" up -d --force-recreate --remove-orphans

  echo "Sleeping for $BENCH_RUN_FOR"

  # unix timestamp in ms
  start=$(date +%s%3N)
  sleep "$BENCH_RUN_FOR"
  end=$(date +%s%3N)

  # TODO(eh-am): use docker-compose exec
  echo "generating meta report"
  docker exec \
    "${PREFIX}_client_1" ./pyrobench report meta \
    --title "Server Pull Benchmark" \
    --params "BENCH_RUN_FOR=$BENCH_RUN_FOR" \
    --params "PYROBENCH_RAND_SEED=$PYROBENCH_RAND_SEED" \
    --params "PYROBENCH_PROFILE_WIDTH=$PYROBENCH_PROFILE_WIDTH"\
    --params "PYROBENCH_PROFILE_DEPTH=$PYROBENCH_PROFILE_DEPTH" \
    --params "PYROBENCH_PROFILE_SYMBOL_LENGTH=$PYROBENCH_PROFILE_SYMBOL_LENGTH" \
    --params "PYROBENCH_APPS=$PYROBENCH_APPS" \
    --params "PYROBENCH_CLIENTS=$PYROBENCH_CLIENTS" \
    --params "PYROBENCH_REQUESTS=$PYROBENCH_REQUESTS" > "$SCRIPT_DIR/meta-report"

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
