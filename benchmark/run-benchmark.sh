#!/usr/bin/env bash

set -euo pipefail

export DOCKER_BUILDKIT=1
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"

cd $SCRIPT_DIR

function composeDown() {
  ./with-env.sh docker-compose down
}

# TODO
# build newest version
./with-env.sh docker-compose build client

# Start the docker containers
./with-env.sh docker-compose up -d --force-recreate --remove-orphans
# cleanup
trap \
  "composeDown" SIGINT SIGTERM ERR EXIT

echo "Generating test load"
# TODO: figure out why it's sending messages
docker exec benchmark_client_1 ./benchmark-main loadgen --log-level=error --server-address="http://pyroscope:4040" > /dev/null &
docker exec benchmark_client_1 ./benchmark-main loadgen --log-level=error --server-address="http://pyroscope_main:4040" > /dev/null &

# unix timestamp in ms
start=$(date +%s%3N)
sleep 10m
#sleep Infinity
end=$(date +%s%3N)

# TODO(eh-am): use docker-compose exec
docker exec benchmark_client_1 ./benchmark-main screenshot-dashboard "$start" "$end" \
  --destination="/screenshots" \
  --grafana-address http://grafana:3000

# get avg rate
docker exec benchmark_client_1 ./benchmark-main \
  'avg(rate(pyroscope_http_request_duration_seconds_count{handler="/ingest"}[5m])) by (instance)' \
  --prometheus-address http://localhost:9091'
