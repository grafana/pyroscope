#!/usr/bin/env bash

set -euo pipefail


docker-compose build

docker-compose up --abort-on-container-exit --remove-orphans
