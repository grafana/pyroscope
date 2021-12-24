#!/bin/bash

set -euo pipefail

source ./config.env
# these env variables are used in docker-compose itself so we're exporting them here
export PYROSCOPE_CPUS PYROSCOPE_MEMORY

export DOCKER_BUILDKIT=1
docker-compose -f docker-compose.yml -f docker-compose.dev.yml up \
  --build \
  --abort-on-container-exit
