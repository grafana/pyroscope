#!/usr/bin/env bash

set -euo pipefail

BENCH_RUN_FOR="${BENCH_RUN_FOR:-10m}"
PYROSCOPE_ADDRESS="http://pyroscope:4040"
PYROSCOPE_MAIN_ADDRESS="http://pyroscope_main:4040"
GRAFANA_ADDRESS="http://grafana:3000"
DASHBOARD_UID="QF9YgRbUbt3BA5Qd"

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
  docker exec pr_client_1 ./pyrobench loadgen --log-level=error --server-address="$PYROSCOPE_ADDRESS" > /dev/null &
  docker exec pr_client_1 ./pyrobench loadgen --log-level=error --server-address="$PYROSCOPE_MAIN_ADDRESS" > /dev/null &

  echo "Sleeping for $BENCH_RUN_FOR"
  # unix timestamp in ms
  start=$(date +%s%3N)
  sleep "$BENCH_RUN_FOR"
  end=$(date +%s%3N)

  echo "Taking screenshots of dashboard $DASHBOARD_UID"
  # TODO(eh-am): use docker-compose exec
  docker exec pr_client_1 ./pyrobench \
    screenshot-dashboard "$start" "$end" \
    --dashboard-uid="$DASHBOARD_UID" \
    --destination="/screenshots" \
    --grafana-address "$GRAFANA_ADDRESS"

  ./generate-report.sh > ./pr-report
}

run
