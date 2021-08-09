#!/bin/bash

set -e

../scripts/generate-git-info.sh > git-info.env

echo "building containers..."

source ./run-parameters.env
export PYROSCOPE_CPUS PYROSCOPE_MEMORY

docker-compose build

NAME="$(date -u +%FT%TZ | tr ':' '-').png"
FROM="$(date +%s000)"

sh -c "nc -l 30014 && node ./take-screenshot.js 'runs/$NAME' '$FROM'" &

ABORT_ON_EXIT="--abort-on-container-exit"
RECREATE_VOLUMES="-V"

SLEEP_PID=""
function cleanup()
{
  echo "cleanup $SLEEP_PID"
  kill $SLEEP_PID
}

trap cleanup EXIT

while [[ $# -gt 0 ]]
do
key="$1"

case $key in
    -w|--wait)
    echo "WAIT=true" >> git-info.env
    ABORT_ON_EXIT=""
    shift # past argument
    ;;
    --keep-data)
    RECREATE_VOLUMES=""
    shift # past argument
    ;;
    --stop-pyroscope-after)
    STOP_AFTER="$2"
    (sleep $STOP_AFTER && docker-compose stop -t 1000 pyroscope) &
    SLEEP_PID=$!
    shift # past argument
    shift # past argument
    ;;
    *)    # unknown option
    shift # past argument
    ;;
esac
done
set -- "${POSITIONAL[@]}" # restore positional parameters


echo args "$RECREATE_VOLUMES" "$ABORT_ON_EXIT"
docker-compose up --remove-orphans $RECREATE_VOLUMES $ABORT_ON_EXIT
