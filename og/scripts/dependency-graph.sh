#!/bin/sh

godepgraph -nostdlib -s ./cmd/pyroscope/ > tmp/deps.dot

(
  cat tmp/deps.dot | head -n 7 | grep -v 'splines=ortho'
  cat tmp/deps.dot | \
    grep pyroscope | \
    sed 's/github.com\/pyroscope-io\/pyroscope\///g' | \
    grep -v 'pkg/config' | \
    grep -v 'pkg/build' | \
    grep -v .com
  echo "}"
) | dot -Tsvg -o tmp/go-deps-graph.svg
