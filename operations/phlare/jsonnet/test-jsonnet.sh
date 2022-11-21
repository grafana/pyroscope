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
jb install github.com/grafana/phlare/operations/phlare@main

# link latest
rm -rf vendor/github.com/grafana/phlare/operations/phlare
ln -fs "${REPO_ROOT}/operations/phlare" vendor/github.com/grafana/phlare/operations/phlare

# create a monolithic environment
tk env add --namespace phlare-mono --server https://localhost:6443 environments/phlare-mono
cat > environments/phlare-mono/main.jsonnet <<EOF
local phlare = import 'phlare/jsonnet/phlare/phlare.libsonnet';

phlare.new(name='mono', overrides={
  namespace: 'phlare',
  values+: {
    phlare+: {
      nameOverride: 'mono',
    },
  },
})
EOF
tk export rendered-mono environments/phlare-mono
ls -l rendered-mono

# create a micro-services environment
tk env add --namespace phlare-micro --server https://localhost:6443 environments/phlare-micro
cat > environments/phlare-micro/main.jsonnet <<EOF
local phlare = import 'phlare/jsonnet/phlare/phlare.libsonnet';
local valuesMicroServices = import 'phlare/jsonnet/values-micro-services.json';

phlare.new(name='micro', overrides={
  namespace: 'phlare-micro',
  values+: valuesMicroServices {
    phlare+: {
      nameOverride: 'micro',
    },
  },
})
EOF
tk export rendered-micro environments/phlare-micro
ls -l rendered-micro

