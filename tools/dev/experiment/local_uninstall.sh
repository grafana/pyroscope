#!/usr/bin/env bash

set -e
set -o pipefail

PYROSCOPE_TEST_NAMESPACE=pyroscope-test

helm -n "$PYROSCOPE_TEST_NAMESPACE" uninstall pyroscope
