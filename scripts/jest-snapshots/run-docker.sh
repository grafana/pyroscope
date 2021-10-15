#!/usr/bin/env bash

set -euo pipefail

#docker run \
#  -it --rm \
#  --entrypoint=/bin/bash \
#  -v $PWD/package.json:/app/package.json \
#  -v $PWD/yarn.lock:/app/yarn.lock \
#  -v $PWD/webapp:/app/webapp \
#  -v $PWD/scripts:/app/scripts \
#  -v $PWD/tsconfig.json:/app/tsconfig.json \
#  -v $PWD/jest.config.js:/app/jest.config.js\
#  -v $PWD/babel.config.js:/app/babel.config.js \
#  -w /app \
#  node:12-buster-slim -- \
#  ./scripts/jest-snapshots/run-snapshots.sh

docker run \
  -it --rm \
  --entrypoint=/bin/bash \
  -v $PWD:/app/:ro \
  -v /app/node_modules \
  -w /app \
  node:current-slim ./scripts/jest-snapshots/run-snapshots.sh
