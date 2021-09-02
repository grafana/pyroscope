#!/usr/bin/env bash

set -euo pipefail

export DOCKER_BUILDKIT=1
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"

cd $SCRIPT_DIR

function composeDown() {
  ./with-env.sh docker-compose down
}

./with-env.sh docker-compose build client

# Start the docker containers
./with-env.sh docker-compose up -d --force-recreate --remove-orphans
# cleanup
trap \
  "./with-env.sh docker-compose down" SIGINT SIGTERM ERR EXIT

go run cmd/main.go loadgen --server-address="http://localhost:4040" &


start=$(date +%s)
#sleep 5m
sleep 30s

#sleep Infinity
# TODO(eh-am): use docker-compose exec
docker exec benchmark_client_1 ./benchmark-main ci-report > report.md
