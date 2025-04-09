#!/usr/bin/env bash

# IMAGE_NAME=$(whoami)/pyroscope IMAGE_TAG=$(./tools/image-tag) ./tools/dev/experiment/local_install.sh

set -x
set -e

PYROSCOPE_TEST_NAMESPACE=pyroscope-test
HELM_CHART=./operations/pyroscope/helm/pyroscope
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VALUES_FILE="$SCRIPT_DIR/values.yaml"

helm -n "$PYROSCOPE_TEST_NAMESPACE" upgrade --install \
  --create-namespace pyroscope \
  --values "$VALUES_FILE" \
  --set pyroscope.image.repository="$IMAGE_NAME" \
  --set pyroscope.image.tag="$IMAGE_TAG" \
  "$HELM_CHART"

sleep 5

kubectl --namespace $PYROSCOPE_TEST_NAMESPACE port-forward svc/pyroscope-query-frontend 4040:4040
