#!/usr/bin/env bash

set -euo pipefail
set -x

# Decide for how long the benchmark will run
BENCH_RUN_FOR="${BENCH_RUN_FOR:-10m}"

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

}

run
