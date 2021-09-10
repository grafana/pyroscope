#!/usr/bin/env bash

set -euo pipefail

BENCH_RUN_FOR="${BENCH_RUN_FOR:-10m}"
PYROSCOPE_ADDRESS="http://pyroscope:4040"
PYROSCOPE_MAIN_ADDRESS="http://pyroscope_main:4040"
PUSHGATEWAY_ADDRESS="http://pushgateway:9091"
PROMETHEUS_ADDRESS="http://prometheus:9090"
GRAFANA_ADDRESS="http://grafana:3000"
DASHBOARD_UID="QF9YgRbUbt3BA5Qd"
UPLOAD_DEST="${GIT_COMMIT:-/screenshots}"
BUCKET_NAME="${BUCKET_NAME:-}"
set +u
[[ -z "$AWS_ACCESS_KEY_ID" ]] && UPLOAD_TYPE="fs" || UPLOAD_TYPE="s3"
set -u

# set as empty string so that reexporting in docker works
AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID:-}"
AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY:-}"

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
  docker exec pr_client_1 ./pyrobench loadgen \
    --log-level=error \
    --server-address="$PYROSCOPE_ADDRESS" \
    --pushgateway-address="$PUSHGATEWAY_ADDRESS" \
    > /dev/null &
  docker exec pr_client_1 ./pyrobench loadgen \
    --log-level=error \
    --server-address="$PYROSCOPE_MAIN_ADDRESS"  \
    --pushgateway-address="$PUSHGATEWAY_ADDRESS" \
    > /dev/null &

  echo "Sleeping for $BENCH_RUN_FOR"
  # unix timestamp in ms
  start=$(date +%s%3N)
  sleep "$BENCH_RUN_FOR"
  end=$(date +%s%3N)


  echo "Taking screenshots of dashboard $DASHBOARD_UID"
  # TODO(eh-am): use docker-compose exec
  docker exec \
    -e "AWS_ACCESS_KEY_ID=$AWS_ACCESS_KEY_ID" \
    -e "AWS_SECRET_ACCESS_KEY=$AWS_SECRET_ACCESS_KEY" \
    pr_client_1 ./pyrobench report image \
    --from="$start" \
    --to="$end" \
    --upload-bucket="$BUCKET_NAME" \
    --upload-dest="$UPLOAD_DEST" \
    --grafana-address "$GRAFANA_ADDRESS" \
    --upload-type="$UPLOAD_TYPE" > "$SCRIPT_DIR/image-report"

  docker exec \
    -e "AWS_ACCESS_KEY_ID=$AWS_ACCESS_KEY_ID" \
    -e "AWS_SECRET_ACCESS_KEY=$AWS_SECRET_ACCESS_KEY" \
    pr_client_1 ./pyrobench report table \
    --prometheus-address="$PROMETHEUS_ADDRESS" \
    --queries-file /report.tmpl.yaml > "$SCRIPT_DIR/table-report"
}

run
