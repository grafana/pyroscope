#!/usr/bin/env bash

set -euo pipefail


docker run \
  --net=host \ # since cypress will hit http://localhost:4040
  -it --rm \
  -e CYPRESS_COMPARE_SNAPSHOTS=true \
  -e CYPRESS_updateSnapshots=true \
  -v $PWD:/cypress -w /cypress cypress/included:8.4.1

