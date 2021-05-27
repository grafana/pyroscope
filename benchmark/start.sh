#!/bin/bash

set -e

../scripts/generate-git-info.sh > git-info.env

echo "building containers..."
docker-compose build


NAME="$(date -u +%FT%TZ | tr ':' '-').png"
FROM="$(date +%s000)"

sh -c "nc -l 30014 && node ./take-screenshot.js 'runs/$NAME' '$FROM'" &

if [ $1 = "--wait" ]; then
  docker-compose up -V
else
  docker-compose up -V --abort-on-container-exit
fi
