#!/bin/bash

set -e

echo "building containers..."

source ./run-parameters.env
export PYROSCOPE_CPUS PYROSCOPE_MEMORY
export DOCKER_BUILDKIT=1
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
LOCKFILE=$SCRIPT_DIR/lock

trap 'rc=$?; echo "ERR at line ${LINENO} (rc: $rc)"; cleanup; exit $rc' ERR
trap 'rc=$?; echo "EXIT (rc: $rc)"; cleanup; exit $rc' EXIT

wait=false

function cleanup() {
  lockContent=`cat $LOCKFILE`
  if [[ "$lockContent" -eq "$$" ]]; then
    echo "Removing lock file..."
    rm "$LOCKFILE"
  fi

  # TODO(eh-am): what to do when this fails?
  docker-compose down --remove-orphans
}

function run() {
  docker-compose build > /dev/null
  docker-compose up -d \
    --remove-orphans

  # TODO(eh-am): docker-compose exec
  docker exec benchmark_pyrobench_1 ./pyrobench loadgen \
    --server-address="http://pyroscope:4040"

  if [ "$wait" = true ]; then
    echo "Finished, waiting since --wait flag is true"
    sleep 24h
  fi

}

if [ -f "$LOCKFILE" ]; then
  echo "Already running... will now terminate."
  exit
else
  echo "Acquiring lock..."
  echo $$ > "$LOCKFILE"
fi

while [[ $# -gt 0 ]]; do
  key="$1"

  case $key in
      -w|--wait)
      wait=true
      shift # past argument
      ;;
      *)    # unknown option
      shift # past argument
      ;;
  esac
done
set -- "${POSITIONAL[@]}" # restore positional parameters

run
