#!/usr/bin/env bash

set -x
set -e

if [ -z "$DOCKER_REPO" ]; then
    echo "Specify the docker repository using the DOCKER_REPO variable."
    exit 1
fi

IMAGE_NAME=$DOCKER_REPO/pyroscope
PYROSCOPE_TEST_NAMESPACE=pyroscope-test
HELM_CHART=./operations/pyroscope/helm/pyroscope
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VALUES_FILE="$SCRIPT_DIR/values-micro-services-experiment.yaml"

# build and push image

GOOS=linux GOARCH=amd64 IMAGE_PREFIX="$DOCKER_REPO/" make docker-image/pyroscope/push

IMAGE_TAG=$(docker image ls | grep "$IMAGE_NAME" | head -n 1 | awk '{print $2}')
if [ -z "$IMAGE_TAG" ]; then
    echo "Error: can't find image tag for $IMAGE_NAME."
    exit 1
fi

echo "using image: $IMAGE_NAME:$IMAGE_TAG"

# deploy

helm -n "$PYROSCOPE_TEST_NAMESPACE" upgrade --install \
  --create-namespace pyroscope \
  --values "$VALUES_FILE" \
  --set pyroscope.image.repository="$IMAGE_NAME" \
  --set pyroscope.image.tag="$IMAGE_TAG" \
  "$HELM_CHART"

kubectl --namespace "$PYROSCOPE_TEST_NAMESPACE" port-forward svc/pyroscope-query-frontend 4040:4040
