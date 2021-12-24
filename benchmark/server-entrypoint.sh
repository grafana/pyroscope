#!/bin/bash

cat /tmp/server.yml.example | grep -B 1000 'static-configs:' > /tmp/server.yml

PYROBENCH_PULL_TARGETS="${PYROBENCH_PULL_TARGETS:-10}"

echo "starting pyroscope with $PYROBENCH_PULL_TARGETS pull targets"

for ((x=0 ; x<$PYROBENCH_PULL_TARGETS ; x++)); do
  echo "      - application: pyrobench" >> /tmp/server.yml
  echo "        targets:"                 >> /tmp/server.yml
  echo "          - pyrobench:4042"     >> /tmp/server.yml
  echo "        labels:"                  >> /tmp/server.yml
  echo "          pod: pod-$x"            >> /tmp/server.yml
done

pyroscope server -config /tmp/server.yml
