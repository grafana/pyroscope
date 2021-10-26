#!/bin/sh

godepgraph -s ./pkg/agent/profiler > tmp/deps.dot

(
  cat tmp/deps.dot | head -n 7 | grep -v 'splines=ortho'
  cat tmp/deps.dot | \
    grep pyroscope | \
    sed 's/github.com\/pyroscope-io\/pyroscope\///g'
  echo "}"
) | tee tmp/log.txt | dot -Tsvg -o tmp/client-deps-graph.svg

rm tmp/deps.dot
