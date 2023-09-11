#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
declare -r REPO_ROOT
cd "${REPO_ROOT}"

# create temporary directory
TMP_DIR="$(mktemp -d)"
declare -r TMP_DIR
on_exit() {
  rm -rf "${TMP_DIR}"
}
trap on_exit EXIT

cd "${TMP_DIR}"
tk init

jb install github.com/grafana/jsonnet-libs/tanka-util@master
jb install github.com/grafana/pyroscope/operations/pyroscope@main

# link latest
rm -rf vendor/github.com/grafana/pyroscope/operations/pyroscope
ln -fs "${REPO_ROOT}/operations/pyroscope" vendor/github.com/grafana/pyroscope/operations/pyroscope

# create a monolithic environment
tk env add --namespace pyroscope-mono --server https://localhost:6443 environments/pyroscope-mono
cat > environments/pyroscope-mono/main.jsonnet <<EOF
local pyroscope = import 'pyroscope/jsonnet/pyroscope/pyroscope.libsonnet';

pyroscope.new(name='mono', overrides={
  namespace: 'pyroscope',
  values+: {
    pyroscope+: {
      nameOverride: 'mono',
    },
  },
})
EOF
tk export rendered-mono environments/pyroscope-mono
ls -l rendered-mono

# create a micro-services environment
tk env add --namespace pyroscope-micro --server https://localhost:6443 environments/pyroscope-micro
cat > environments/pyroscope-micro/main.jsonnet <<EOF
local pyroscope = import 'pyroscope/jsonnet/pyroscope/pyroscope.libsonnet';
local valuesMicroServices = import 'pyroscope/jsonnet/values-micro-services.json';

pyroscope.new(name='micro', overrides={
  namespace: 'pyroscope-micro',
  values+: valuesMicroServices {
    pyroscope+: {
      nameOverride: 'micro',
    },
  },
})
EOF
tk export rendered-micro environments/pyroscope-micro
ls -l rendered-micro

